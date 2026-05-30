package localworkspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Repository[T any] struct {
	Workspace  Workspace
	Collection string
	FileName   string
	Kind       string
	IDPrefix   string
	Now        func() time.Time
}

func (r Repository[T]) New(data T) (Envelope[T], error) {
	if err := r.validateConfig(); err != nil {
		return Envelope[T]{}, err
	}
	now := r.now()
	id, err := NewID(r.IDPrefix, now)
	if err != nil {
		return Envelope[T]{}, err
	}
	timestamp := now.UTC().Format(time.RFC3339)
	return Envelope[T]{
		SchemaVersion: SchemaVersion,
		Kind:          r.Kind,
		ID:            id,
		Revision:      1,
		CreatedAt:     timestamp,
		UpdatedAt:     timestamp,
		Data:          data,
	}, nil
}

func (r Repository[T]) Read(id string) (Envelope[T], error) {
	if err := r.validateConfig(); err != nil {
		return Envelope[T]{}, err
	}
	if err := r.validateID(id); err != nil {
		return Envelope[T]{}, err
	}
	data, err := os.ReadFile(r.FilePath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return Envelope[T]{}, NewError(ErrorWorkspaceNotFound, "object document not found", 2, map[string]string{"id": id})
		}
		return Envelope[T]{}, WrapError(ErrorInternal, "read object document", 5, err)
	}
	var document Envelope[T]
	if err := json.Unmarshal(data, &document); err != nil {
		return Envelope[T]{}, WrapError(ErrorWorkspaceInvalid, "parse object document", 2, err)
	}
	if err := r.validateDocument(document); err != nil {
		return Envelope[T]{}, err
	}
	return document, nil
}

func (r Repository[T]) Write(document Envelope[T]) error {
	if err := r.validateConfig(); err != nil {
		return err
	}
	if err := r.validateDocument(document); err != nil {
		return err
	}
	return AtomicWriteJSON(r.FilePath(document.ID), document, 0o600)
}

func (r Repository[T]) Delete(id string) error {
	if err := r.validateConfig(); err != nil {
		return err
	}
	if err := r.validateID(id); err != nil {
		return err
	}
	if _, err := os.Stat(r.FilePath(id)); err != nil {
		if os.IsNotExist(err) {
			return NewError(ErrorWorkspaceNotFound, "object document not found", 2, map[string]string{"id": id})
		}
		return WrapError(ErrorInternal, "stat object document", 5, err)
	}
	if err := os.RemoveAll(r.Dir(id)); err != nil {
		return WrapError(ErrorInternal, "delete object directory", 5, err)
	}
	return nil
}

func (r Repository[T]) List() ([]Envelope[T], error) {
	if err := r.validateConfig(); err != nil {
		return nil, err
	}
	dir := filepath.Join(r.Workspace.Root, r.Collection)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, NewError(ErrorWorkspaceInvalid, "collection directory not found", 2, map[string]string{"collection": r.Collection})
		}
		return nil, WrapError(ErrorInternal, "read collection directory", 5, err)
	}
	sort.Slice(entries, func(i int, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})
	documents := []Envelope[T]{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		document, err := r.Read(entry.Name())
		if err != nil {
			return nil, err
		}
		documents = append(documents, document)
	}
	return documents, nil
}

func (r Repository[T]) Dir(id string) string {
	return filepath.Join(r.Workspace.Root, r.Collection, id)
}

func (r Repository[T]) FilePath(id string) string {
	return filepath.Join(r.Dir(id), r.FileName)
}

func (r Repository[T]) validateConfig() error {
	if strings.TrimSpace(r.Workspace.Root) == "" {
		return NewError(ErrorInvalidArgument, "repository workspace root is empty", 1, nil)
	}
	if err := validatePathComponent("repository collection", r.Collection); err != nil {
		return err
	}
	if err := validatePathComponent("repository file name", r.FileName); err != nil {
		return err
	}
	if strings.TrimSpace(r.Kind) == "" {
		return NewError(ErrorInvalidArgument, "repository kind is empty", 1, nil)
	}
	if err := validateIDPrefix(r.IDPrefix); err != nil {
		return err
	}
	return nil
}

func (r Repository[T]) validateDocument(document Envelope[T]) error {
	if document.SchemaVersion != SchemaVersion {
		return NewError(ErrorWorkspaceInvalid, "object schema version mismatch", 2, map[string]string{"schemaVersion": document.SchemaVersion})
	}
	if document.Kind != r.Kind {
		return NewError(ErrorWorkspaceInvalid, "object kind mismatch", 2, map[string]string{"kind": document.Kind})
	}
	if err := r.validateID(document.ID); err != nil {
		return err
	}
	if document.Revision < 1 {
		return NewError(ErrorWorkspaceInvalid, "object revision must be at least 1", 2, map[string]string{"id": document.ID})
	}
	if _, err := time.Parse(time.RFC3339, document.CreatedAt); err != nil {
		return WrapError(ErrorWorkspaceInvalid, "parse object createdAt", 2, err)
	}
	if _, err := time.Parse(time.RFC3339, document.UpdatedAt); err != nil {
		return WrapError(ErrorWorkspaceInvalid, "parse object updatedAt", 2, err)
	}
	return nil
}

func (r Repository[T]) validateID(id string) error {
	if strings.TrimSpace(id) == "" {
		return NewError(ErrorInvalidArgument, "object id is empty", 1, nil)
	}
	if err := validatePathComponent("object id", id); err != nil {
		return err
	}
	if !strings.HasPrefix(id, r.IDPrefix+"_") {
		return NewError(ErrorWorkspaceInvalid, "object id prefix mismatch", 2, map[string]string{"id": id})
	}
	return nil
}

func (r Repository[T]) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

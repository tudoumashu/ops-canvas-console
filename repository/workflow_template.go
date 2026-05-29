package repository

import (
	"github.com/basketikun/infinite-canvas/model"
)

func ListWorkflowTemplates(workflowType string, q model.Query) ([]model.WorkflowTemplate, int64, error) {
	db, err := DB()
	if err != nil {
		return nil, 0, err
	}
	q.Normalize()
	tx := db.Model(&model.WorkflowTemplate{}).Where("workflow_type = ?", workflowType)
	if q.Keyword != "" {
		like := "%" + q.Keyword + "%"
		tx = tx.Where("title LIKE ? OR description LIKE ?", like, like)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []model.WorkflowTemplate
	err = tx.Order("updated_at desc").Offset(q.Offset()).Limit(q.PageSize).Find(&items).Error
	return items, total, err
}

func GetWorkflowTemplate(id string) (model.WorkflowTemplate, error) {
	db, err := DB()
	if err != nil {
		return model.WorkflowTemplate{}, err
	}
	var item model.WorkflowTemplate
	err = db.Where("id = ?", id).First(&item).Error
	return item, err
}

func SaveWorkflowTemplate(item model.WorkflowTemplate) (model.WorkflowTemplate, error) {
	db, err := DB()
	if err != nil {
		return item, err
	}
	return item, db.Save(&item).Error
}

func DeleteWorkflowTemplate(id string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Delete(&model.WorkflowTemplate{}, "id = ?", id).Error
}

func ListWorkflowRuns(workflowType string, q model.Query) ([]model.WorkflowRun, int64, error) {
	db, err := DB()
	if err != nil {
		return nil, 0, err
	}
	q.Normalize()
	tx := db.Model(&model.WorkflowRun{}).Where("workflow_type = ?", workflowType)
	if q.Keyword != "" {
		like := "%" + q.Keyword + "%"
		tx = tx.Where("id LIKE ? OR template_title LIKE ? OR error LIKE ?", like, like, like)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []model.WorkflowRun
	err = tx.Order("updated_at desc").Offset(q.Offset()).Limit(q.PageSize).Find(&items).Error
	return items, total, err
}

func GetWorkflowRun(id string) (model.WorkflowRun, error) {
	db, err := DB()
	if err != nil {
		return model.WorkflowRun{}, err
	}
	var item model.WorkflowRun
	err = db.Where("id = ?", id).First(&item).Error
	return item, err
}

func SaveWorkflowRun(item model.WorkflowRun) (model.WorkflowRun, error) {
	db, err := DB()
	if err != nil {
		return item, err
	}
	return item, db.Save(&item).Error
}

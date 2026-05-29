import PDDWorkflowTemplateEditor from "./template-editor";

export default async function Page({ params }: { params: Promise<{ templateId: string }> }) {
    const { templateId } = await params;
    return <PDDWorkflowTemplateEditor templateId={decodeURIComponent(templateId)} />;
}

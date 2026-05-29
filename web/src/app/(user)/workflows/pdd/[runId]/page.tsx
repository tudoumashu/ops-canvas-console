import PDDRunClientPage from "./pdd-workflow-client-page";

export default async function PDDRunPage({ params }: { params: Promise<{ runId: string }> }) {
    const { runId } = await params;
    return <PDDRunClientPage runId={decodeURIComponent(runId)} />;
}

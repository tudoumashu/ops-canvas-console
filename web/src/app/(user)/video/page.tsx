import { redirect } from "next/navigation";

export default function VideoPage() {
    redirect("/workbench?mode=video");
}

import { redirect } from "next/navigation";

export default function ImagePage() {
    redirect("/workbench?mode=image");
}

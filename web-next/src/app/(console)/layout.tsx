import type { ReactNode } from "react";
import { Header } from "@/shared/ui/Header";
import { Sidebar } from "@/shared/ui/Sidebar";

export default function ConsoleLayout({ children }: { children: ReactNode }) {
  return (
    <div className="flex min-h-screen">
      <Sidebar />
      <div className="flex min-w-0 flex-1 flex-col">
        <Header />
        <main className="flex-1 px-8 py-7">{children}</main>
      </div>
    </div>
  );
}

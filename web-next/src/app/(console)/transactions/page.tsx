import { releaseFor } from "@/shared/navigation/nav-items";
import { ComingSoon } from "@/shared/ui/ComingSoon";

export default function Page() {
  return <ComingSoon release={releaseFor("/transactions")} />;
}

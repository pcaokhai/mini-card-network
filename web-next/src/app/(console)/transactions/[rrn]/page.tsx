import { JourneyScreen } from "./JourneyScreen";

export default async function Page({ params }: { params: Promise<{ rrn: string }> }) {
  const { rrn } = await params;
  return <JourneyScreen rrn={rrn} />;
}

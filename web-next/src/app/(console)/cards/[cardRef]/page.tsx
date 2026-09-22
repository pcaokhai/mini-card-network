import { CardDetailScreen } from "./CardDetailScreen";

export default async function Page({ params }: { params: Promise<{ cardRef: string }> }) {
  const { cardRef } = await params;
  return <CardDetailScreen cardRef={cardRef} />;
}

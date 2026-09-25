import { CardsScreen } from "../CardsScreen";

export default async function Page({ params }: { params: Promise<{ cardRef: string }> }) {
  const { cardRef } = await params;
  return <CardsScreen cardRef={cardRef} />;
}

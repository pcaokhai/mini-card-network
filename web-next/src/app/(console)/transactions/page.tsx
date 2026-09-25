import { Suspense } from "react";
import { JourneyIndexScreen } from "./JourneyIndexScreen";

// useSearchParams needs a Suspense boundary to render the rest of the route statically.
export default function Page() {
  return (
    <Suspense>
      <JourneyIndexScreen />
    </Suspense>
  );
}

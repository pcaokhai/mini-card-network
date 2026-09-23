import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { NextIntlClientProvider } from "next-intl";
import { HttpResponse, http } from "msw";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";
import en from "../../../messages/en.json";
import { RotationStepper } from "./RotationStepper";

// Fixed shape so the test isn't at the mercy of the generated handlers' random
// step arrays (they don't guarantee all four named steps appear).
const ROTATION: import("@/shared/api/security-client").KeyRotation = {
  rotationId: "rot_1",
  keyType: "ZPK",
  status: "COMPLETED",
  newKcv: "ABC123",
  steps: [
    { name: "GENERATE", status: "DONE", completedAt: null },
    { name: "SEND_0800_161", status: "DONE", completedAt: null },
    { name: "PARTNER_CONFIRM", status: "DONE", completedAt: null },
    { name: "ACTIVATE", status: "DONE", completedAt: null },
  ],
};

const server = setupServer(
  http.post("/api/v1/keys/acquirer/rotations", () => HttpResponse.json(ROTATION, { status: 202 })),
  http.get("/api/v1/keys/acquirer/rotations/:rotationId", () => HttpResponse.json(ROTATION)),
);
beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

function renderStepper() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <NextIntlClientProvider locale="en" messages={en}>
        <RotationStepper keyType="ZPK" />
      </NextIntlClientProvider>
    </QueryClientProvider>,
  );
}

describe("RotationStepper", () => {
  it("starts a rotation and renders the four named steps__MCN_505_AC1", async () => {
    renderStepper();
    await userEvent.click(screen.getByRole("button", { name: /rotate/i }));

    await waitFor(() => {
      expect(screen.getByText(/GENERATE/)).toBeInTheDocument();
      expect(screen.getByText(/SEND_0800_161/)).toBeInTheDocument();
      expect(screen.getByText(/PARTNER_CONFIRM/)).toBeInTheDocument();
      expect(screen.getByText(/ACTIVATE/)).toBeInTheDocument();
    });
  });
});

import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, delay, http } from "msw";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";
import { renderWithIntl } from "@/test/render";
import type { Transaction } from "@/shared/api/pos-client";
import { PosScreen } from "./PosScreen";

const approved: Transaction = {
  rrn: "626807000301",
  stan: "000301",
  type: "PURCHASE",
  status: "APPROVED",
  responseCode: "00",
  authCode: "A00301",
  amount: { amount: 250_000, currency: "704" },
  maskedPan: "970436******4417",
  terminalId: "00000042",
  merchantName: "Cà phê Góc Phố",
  createdAt: "2026-09-25T07:00:00Z",
};

let bodies: { path: string; body: Record<string, unknown> }[] = [];

function answer(type: Transaction["type"], waitMs = 0) {
  return async ({ request }: { request: Request }) => {
    bodies.push({ path: new URL(request.url).pathname, body: (await request.json()) as Record<string, unknown> });
    await delay(waitMs);
    return HttpResponse.json({ ...approved, type }, { status: 201 });
  };
}

const server = setupServer(
  http.get("*/v1/cards/:cardRef", () => new HttpResponse(null, { status: 404 })),
  http.post("*/v1/transactions/purchases", answer("PURCHASE", 30)),
  http.post("*/v1/transactions/pre-authorizations", answer("PREAUTH")),
  http.post("*/v1/transactions/refunds", answer("REFUND")),
  http.post("*/v1/transactions/balance-inquiries", answer("BALANCE")),
  http.post("*/v1/transactions/:rrn/completions", answer("COMPLETION")),
);
beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => {
  bodies = [];
});
afterAll(() => server.close());

const pay = () => screen.getByRole("button", { name: /^(Thanh toán|Đang xử lý…)$/ });

describe("PosScreen", () => {
  it("has a single heading naming the screen and starts on the canvas's default sale", () => {
    renderWithIntl(<PosScreen />);
    expect(screen.getByRole("heading", { level: 1, name: "Máy POS giả lập" })).toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent("250.000 ₫");
    expect(screen.getByRole("button", { name: /4417/ })).toHaveAttribute("aria-pressed", "true");
  });

  it("MCN-305-AC2 locks Pay and shows the processing state until the API answers", async () => {
    const user = userEvent.setup();
    renderWithIntl(<PosScreen />);
    await user.click(pay());

    expect(pay()).toBeDisabled();
    expect(pay()).toHaveTextContent("Đang xử lý…");
    expect(screen.getByRole("status")).toHaveTextContent("Đang xử lý");
    expect(await screen.findByText("Thanh toán thành công")).toBeInTheDocument();
    expect(pay()).not.toBeDisabled();
    expect(screen.getByRole("status")).toHaveTextContent("Giao dịch thành công");
  });

  it("MCN-305-AC4 (Ruling R1) sends a chip read with no PIN block", async () => {
    const user = userEvent.setup();
    renderWithIntl(<PosScreen />);
    await user.click(pay());
    await screen.findByText("Thanh toán thành công");

    expect(bodies[0]?.body).toEqual({
      terminalId: "00000042",
      cardToken: "tok_normal",
      entryMode: "CHIP_NO_PIN",
      amount: { amount: 250_000, currency: "704" },
    });
  });

  it("MCN-305-AC1 manual entry, keypad digits and a scenario preset shape the request", async () => {
    const user = userEvent.setup();
    renderWithIntl(<PosScreen />);
    await user.click(screen.getByRole("button", { name: "Không đủ tiền" }));
    expect(screen.getByRole("status")).toHaveTextContent("350.000 ₫");
    await user.click(screen.getByRole("button", { name: "Nhập tay" }));
    await user.click(screen.getByRole("button", { name: "Xóa một số" }));
    await user.click(pay());
    await screen.findByText("Thanh toán thành công");

    expect(bodies[0]?.body).toMatchObject({ cardToken: "tok_low", entryMode: "MANUAL_NO_PIN", amount: { amount: 35_000 } });
  });

  it("MCN-604 completes a pre-auth by its original RRN", async () => {
    const user = userEvent.setup();
    renderWithIntl(<PosScreen />);
    await user.click(screen.getByRole("button", { name: "Hoàn tất" }));
    expect(pay()).toBeDisabled();
    await user.type(screen.getByLabelText("Mã tra soát gốc"), "626807000290");
    await user.click(pay());
    await screen.findByText("Đã hoàn tất giao dịch");

    expect(bodies[0]).toEqual({
      path: "/api/v1/transactions/626807000290/completions",
      body: { amount: { amount: 250_000, currency: "704" } },
    });
  });

  it("MCN-604 a balance inquiry sends no amount", async () => {
    const user = userEvent.setup();
    renderWithIntl(<PosScreen />);
    await user.click(screen.getByRole("button", { name: "Tra cứu số dư" }));
    await user.click(pay());
    await screen.findByText("Tra cứu số dư thành công");

    expect(bodies[0]?.path).toBe("/api/v1/transactions/balance-inquiries");
    expect(bodies[0]?.body).not.toHaveProperty("amount");
  });

  it("an amount of 0 is refused on the terminal without sending anything", async () => {
    const user = userEvent.setup();
    renderWithIntl(<PosScreen />);
    await user.click(screen.getByRole("button", { name: "Xóa hết" }));
    await user.click(pay());

    expect(screen.getByText("Số tiền chưa hợp lệ")).toBeInTheDocument();
    await waitFor(() => expect(screen.getByRole("status")).toHaveTextContent("Nhập số tiền"));
    expect(bodies).toHaveLength(0);
  });
});

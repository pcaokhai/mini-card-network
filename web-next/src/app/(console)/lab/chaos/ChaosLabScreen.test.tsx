import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http, ws } from "msw";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from "vitest";
import { renderWithIntl } from "@/test/render";
import { handlers } from "@/mocks/generated/handlers";
import { scenarioHandlers } from "@/mocks/scenario-handlers";
import { chaosHandlers, resetChaosMock } from "@/mocks/pages/chaos";
import { useDisplayMode } from "@/shared/state/display-mode";
import { ChaosLabScreen } from "./ChaosLabScreen";

const stream = ws.link(/\/v1\/stream$/);
const server = setupServer(...chaosHandlers, ...scenarioHandlers, ...handlers, stream.addEventListener("connection", () => {}));
beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
beforeEach(() => {
  resetChaosMock();
  useDisplayMode.setState({ mode: "easy" });
});
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

describe("ChaosLabScreen", () => {
  it("renders six scenario cards, the active one highlighted, and the active-count pill __MCN_405_AC1", async () => {
    renderWithIntl(<ChaosLabScreen />);
    expect(await screen.findByText("1 sự cố đang bật")).toBeInTheDocument();
    expect(screen.getAllByRole("button", { pressed: false, name: "Bật sự cố" })).toHaveLength(5);
    expect(screen.getByTestId("scenario-card-DROP_RESPONSE")).toHaveAttribute("data-on", "true");
    expect(screen.getByRole("heading", { level: 1, name: "Phòng thí nghiệm sự cố" })).toBeInTheDocument();
  });

  it("turns a scenario on and lists its reaction __MCN_405_AC1_AC3", async () => {
    renderWithIntl(<ChaosLabScreen />);
    await screen.findByText("1 sự cố đang bật");
    await userEvent.click(within(screen.getByTestId("scenario-card-SLOW_NETWORK")).getByRole("button"));

    expect(await screen.findByText("2 sự cố đang bật")).toBeInTheDocument();
    const reactions = screen.getByRole("heading", { name: "Hệ thống đang phản ứng thế nào" }).closest("section") as HTMLElement;
    expect(within(reactions).getAllByRole("listitem")).toHaveLength(2);
  });

  it("turns every scenario off and shows the calm state __MCN_405_AC1_AC3", async () => {
    renderWithIntl(<ChaosLabScreen />);
    await screen.findByText("1 sự cố đang bật");
    await userEvent.click(screen.getByRole("button", { name: "Tắt tất cả" }));

    expect(await screen.findByText("Mọi thứ bình thường")).toBeInTheDocument();
    expect(screen.getByText(/Mọi thứ đang bình thường/)).toBeInTheDocument();
  });

  it("runs 100 test transactions and flashes the zero discrepancy when the run passes __MCN_405_AC2", async () => {
    let posted: unknown;
    server.use(
      http.post("*/v1/chaos/runs", async ({ request }) => {
        posted = await request.json();
        return HttpResponse.json({ runId: "run-9", status: "RUNNING", requested: 100, completed: 0, openingBalanceTotal: 500_000_000 }, { status: 202 });
      }),
      http.get("*/v1/chaos/runs/:runId", () =>
        HttpResponse.json({
          runId: "run-9",
          status: "PASSED",
          requested: 100,
          completed: 100,
          approved: 86,
          declined: 6,
          reversed: 8,
          openingBalanceTotal: 500_000_000,
          closingBalanceTotal: 484_090_000,
          ledgerDiscrepancy: 0,
        }),
      ),
    );
    renderWithIntl(<ChaosLabScreen />);
    await screen.findByText("1 sự cố đang bật");
    expect(screen.getByTestId("money-verification")).toHaveAttribute("data-result", "none");

    await userEvent.click(screen.getByRole("button", { name: "Chạy thử 100 giao dịch" }));

    await waitFor(() => expect(screen.getByTestId("money-verification")).toHaveAttribute("data-result", "ok"));
    expect(posted).toEqual({ transactions: 100 });
    expect(screen.getByTestId("ledger-discrepancy-value")).toHaveAttribute("data-flash", "true");
    expect(screen.getByText("86 giao dịch được duyệt")).toBeInTheDocument();
  });

  it("shows an alert when a toggle fails __MCN_405_AC1", async () => {
    server.use(http.put("*/v1/chaos/scenarios/:scenarioId", () => HttpResponse.json({ title: "boom" }, { status: 500 })));
    renderWithIntl(<ChaosLabScreen />);
    await screen.findByText("1 sự cố đang bật");
    await userEvent.click(within(screen.getByTestId("scenario-card-SLOW_NETWORK")).getByRole("button"));
    expect(await screen.findByRole("alert")).toHaveTextContent("Không đổi được sự cố. Hãy thử lại.");
  });

  it("shows the technical copy in expert mode __MCN_405_AC1_AC3", async () => {
    useDisplayMode.setState({ mode: "expert" });
    renderWithIntl(<ChaosLabScreen />);
    await screen.findByText("1 sự cố đang bật");
    expect(screen.getAllByText("Drop 0210 → timeout → 0420")).toHaveLength(1);
    expect(screen.getByText("0200 timeout → 0420 field 90 → 0430")).toBeInTheDocument();
    expect(screen.getByText("SAF depth")).toBeInTheDocument();
  });

  const baseRun = { requested: 100, openingBalanceTotal: 500_000_000 };

  it("stops polling and frees the run button when the run is gone (gateway restarted) __CHA_G7", async () => {
    let polls = 0;
    server.use(
      http.post("*/v1/chaos/runs", () => HttpResponse.json({ ...baseRun, runId: "run-lost", status: "RUNNING", completed: 0, seq: 0 }, { status: 202 })),
      http.get("*/v1/chaos/runs/:runId", () => {
        polls += 1;
        return HttpResponse.json({ type: "not-found", title: "not-found", status: 404 }, { status: 404 });
      }),
    );
    renderWithIntl(<ChaosLabScreen />);
    await screen.findByText("1 sự cố đang bật");
    const runButton = screen.getByRole("button", { name: "Chạy thử 100 giao dịch" });

    await userEvent.click(runButton);

    await waitFor(() => expect(screen.getByTestId("money-verification")).toHaveAttribute("data-result", "none"));
    expect(runButton).toBeEnabled();
    const settled = polls;
    await new Promise((resolve) => setTimeout(resolve, 2500));
    expect(polls).toBe(settled);
  }, 10_000);

  it("shows an error, not a calm state, when the scenarios can't be loaded __CHA_G10", async () => {
    server.use(http.get("*/v1/chaos/scenarios", () => HttpResponse.json({ title: "boom" }, { status: 500 })));
    renderWithIntl(<ChaosLabScreen />);

    expect(await screen.findByRole("alert")).toHaveTextContent("Không tải được danh sách sự cố. Hãy thử lại.");
    expect(screen.queryByText("Mọi thứ bình thường")).not.toBeInTheDocument();
    expect(screen.queryByTestId("scenario-card-SLOW_NETWORK")).not.toBeInTheDocument();
  });

  it("disables the scenario the stack reports unavailable __CHA_G11", async () => {
    server.use(
      http.get("*/v1/chaos/scenarios", () =>
        HttpResponse.json(
          ["SLOW_NETWORK", "CONNECTION_CUT", "DROP_RESPONSE", "DUPLICATE_REQUEST", "ISSUER_DOWN", "LATE_RESPONSE"].map((id) => ({
            id,
            enabled: false,
            available: id !== "DROP_RESPONSE",
            easyText: id,
            technicalText: id,
          })),
        ),
      ),
    );
    renderWithIntl(<ChaosLabScreen />);

    expect(await screen.findByText("Mọi thứ bình thường")).toBeInTheDocument();
    expect(within(screen.getByTestId("scenario-card-DROP_RESPONSE")).getByRole("button")).toBeDisabled();
    expect(within(screen.getByTestId("scenario-card-SLOW_NETWORK")).getByRole("button")).toBeEnabled();
  });

  it("ignores a chaos.run.progress snapshot older than the one on screen __CHA_G13", async () => {
    const newer = { ...baseRun, runId: "run-ws", status: "RUNNING", completed: 70, approved: 60, seq: 7 };
    const older = { ...newer, completed: 40, approved: 35, seq: 4 };
    server.use(
      http.post("*/v1/chaos/runs", () => HttpResponse.json({ ...baseRun, runId: "run-ws", status: "RUNNING", completed: 0, seq: 0 }, { status: 202 })),
      http.get("*/v1/chaos/runs/:runId", () => HttpResponse.json(newer)),
      stream.addEventListener("connection", ({ client }) => {
        setTimeout(() => client.send(JSON.stringify({ id: "e1", type: "chaos.run.progress", occurredAt: new Date().toISOString(), data: older })), 300);
      }),
    );
    renderWithIntl(<ChaosLabScreen />);
    await screen.findByText("1 sự cố đang bật");

    await userEvent.click(screen.getByRole("button", { name: "Chạy thử 100 giao dịch" }));
    expect(await screen.findByText("60 giao dịch được duyệt")).toBeInTheDocument();
    await new Promise((resolve) => setTimeout(resolve, 500));
    expect(screen.getByText("60 giao dịch được duyệt")).toBeInTheDocument();
    expect(screen.queryByText("35 giao dịch được duyệt")).not.toBeInTheDocument();
  });
});

import { act, fireEvent, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from "vitest";
import { cardsHandlers, resetCardsMock } from "@/mocks/pages/cards";
import { useDisplayMode } from "@/shared/state/display-mode";
import { renderWithIntl } from "@/test/render";
import { CardsScreen } from "./CardsScreen";

const server = setupServer(...cardsHandlers);
beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
beforeEach(() => {
  resetCardsMock();
  useDisplayMode.setState({ mode: "easy" });
});
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

const region = (name: string) => screen.getByRole("region", { name });

async function openCard(cardRef?: string) {
  renderWithIntl(<CardsScreen cardRef={cardRef} />);
  return screen.findByRole("region", { name: "Số dư" });
}

describe("CardsScreen (MCN-309, as in Cards.dc.html)", () => {
  it("lists every card with its holder, last four digits and a status badge MCN-309-AC1", async () => {
    renderWithIntl(<CardsScreen />);
    const list = await screen.findByRole("region", { name: "Thẻ của khách hàng" });
    const items = await within(list).findAllByRole("link");
    // Ordered by last four digits, as the canvas renders them; the issuer's first card is selected.
    expect(items.map((a) => a.textContent?.match(/\d{4}/)?.[0])).toEqual(["1208", "3310", "4417", "5540", "7765", "9021"]);
    expect(items[2]).toHaveTextContent("Nguyễn Minh Anh•••• 4417Đang hoạt động");
    expect(items[2]).toHaveAttribute("aria-current", "page");
    expect(items[2]).toHaveAttribute("href", "/cards/crd_normal0001");
    expect(items[1]).toHaveTextContent("Đã khóa");
    expect(items[4]).toHaveTextContent("Đã hết hạn"); // 08/26 is past, although the issuer still says ACTIVE
  });

  it("shows the card, a three-line balance with its hold, the limits and the ledger MCN-309-AC1", async () => {
    const balance = await openCard("crd_normal0001");

    expect(screen.getByText("•••• •••• •••• 4417")).toBeInTheDocument();
    expect(screen.getByText("Hết hạn 11/28")).toBeInTheDocument();
    expect(await within(balance).findByText("5.000.000 ₫")).toBeInTheDocument();
    expect(within(balance).getByText("−1.500.000 ₫")).toBeInTheDocument();
    expect(within(balance).getByText("3.500.000 ₫")).toBeInTheDocument();
    expect(within(balance).getByText("Tạm giữ cho Khách sạn Hoa Biển · 1.500.000 ₫")).toBeInTheDocument();
    expect(within(balance).getByText("Tự giải phóng 26/09")).toBeInTheDocument();

    const limits = region("Hạn mức");
    expect(within(limits).getByRole("slider", { name: /Hạn mức mỗi ngày/ })).toHaveValue("10000000");
    expect(within(limits).getByText("Đã dùng hôm nay 915.000 ₫ · 9% hạn mức")).toBeInTheDocument();

    const ledger = region("Lịch sử tiền vào, tiền ra");
    const rows = await within(ledger).findAllByRole("row");
    expect(rows).toHaveLength(6); // header + 5 journals
    expect(rows[1]).toHaveTextContent("Mua hàng · Cà phê Góc Phố");
    expect(rows[1]).toHaveTextContent("Tiền ra−250.000 ₫");
    expect(rows[5]).toHaveTextContent("Tiền vào+15.000.000 ₫");
  });

  it("switches every label, badge and ledger leg to the technical names in Expert mode MCN-309-AC1", async () => {
    useDisplayMode.setState({ mode: "expert" });
    const balance = await openCard("crd_normal0001");

    expect(within(balance).getByText("ledger_balance")).toBeInTheDocument();
    expect(within(balance).getByText("auth_hold ACTIVE · Khách sạn Hoa Biển · 1.500.000 ₫")).toBeInTheDocument();
    expect(within(region("Trạng thái thẻ")).getByText("ACTIVE")).toBeInTheDocument();
    expect(screen.getByRole("slider", { name: /card_limit ALL \/ DAILY/ })).toBeInTheDocument();

    const ledger = region("Bút toán kép (journal)");
    const rows = await within(ledger).findAllByRole("row");
    expect(rows[1]).toHaveTextContent("Nợ ACC-crd_normal0001");
    expect(rows[1]).toHaveTextContent("Có SETTLEMENT_SUSPENSE · 250.000");
  });

  it("blocks only after the inline confirmation, then shows the lock overlay and the issuer's audit entry MCN-309-AC2", async () => {
    const user = userEvent.setup();
    await openCard("crd_normal0001");
    const status = region("Trạng thái thẻ");

    await user.click(within(status).getByRole("button", { name: "Khóa thẻ" }));
    expect(within(status).getByText("Khóa thẻ •••• 4417? Mọi giao dịch mới sẽ bị từ chối cho tới khi mở khóa.")).toBeInTheDocument();
    expect(screen.queryByText("Thẻ đang bị khóa")).not.toBeInTheDocument();

    await user.click(within(status).getByRole("button", { name: "Xác nhận khóa" }));
    expect(await screen.findByText("Thẻ đang bị khóa")).toBeInTheDocument();
    expect(await within(status).findByText("Đã khóa")).toBeInTheDocument();
    const audit = within(status).getByRole("list", { name: "Nhật ký thao tác" });
    expect(await within(audit).findByText(/^Khóa thẻ •••• 4417 lúc \d{2}:\d{2} · console$/)).toBeInTheDocument();
    expect(within(status).getByRole("button", { name: "Mở khóa thẻ" })).toBeInTheDocument();
    const list = region("Thẻ của khách hàng");
    expect(await within(list).findByText("Đã khóa", { selector: "a[href='/cards/crd_normal0001'] *" })).toBeInTheDocument();
  });

  it("cancels the confirmation without blocking MCN-309-AC2", async () => {
    const user = userEvent.setup();
    await openCard("crd_normal0001");
    const status = region("Trạng thái thẻ");

    await user.click(within(status).getByRole("button", { name: "Khóa thẻ" }));
    await user.click(within(status).getByRole("button", { name: "Hủy" }));
    expect(within(status).getByText("Đang hoạt động")).toBeInTheDocument();
    expect(within(status).getByRole("button", { name: "Khóa thẻ" })).toBeInTheDocument();
  });

  it("offers no block or unblock on an expired card MCN-309-AC2", async () => {
    await openCard("crd_expird0004");
    const status = region("Trạng thái thẻ");
    expect(await within(status).findByText("Thẻ đã quá ngày hết hạn. Khách cần được phát hành thẻ mới.")).toBeInTheDocument();
    expect(within(status).queryByRole("button")).not.toBeInTheDocument();
  });

  it("sends the card's ETag as If-Match when a slider is released MCN-309-AC3", async () => {
    const sent: { ifMatch: string | null; body: unknown }[] = [];
    server.events.on("request:start", async ({ request }) => {
      if (request.method === "PUT") sent.push({ ifMatch: request.headers.get("If-Match"), body: await request.clone().json() });
    });
    await openCard("crd_normal0001");
    const slider = screen.getByRole("slider", { name: /Hạn mức mỗi ngày/ });

    fireEvent.change(slider, { target: { value: "12000000" } });
    expect(screen.getByText("12.000.000 ₫")).toBeInTheDocument();
    await act(async () => fireEvent.pointerUp(slider));

    await screen.findByText("Đã dùng hôm nay 915.000 ₫ · 8% hạn mức");
    expect(sent).toEqual([{ ifMatch: '"v1"', body: expect.objectContaining({ dailyAmount: { amount: 12_000_000, currency: "704" }, perTransactionAmount: { amount: 5_000_000, currency: "704" } }) }]);
    server.events.removeAllListeners();
  });

  it("shows 'Có người vừa thay đổi thẻ này. Tải lại để tiếp tục.' on a 412 and reloads the card MCN-309-AC3 CARDS-G15", async () => {
    server.use(
      http.put("*/v1/cards/:cardRef/limits", () =>
        HttpResponse.json({ title: "ETag mismatch", status: 412 }, { status: 412, headers: { "Content-Type": "application/problem+json" } }),
      ),
    );
    const user = userEvent.setup();
    await openCard("crd_normal0001");
    const slider = screen.getByRole("slider", { name: /Hạn mức mỗi ngày/ });

    fireEvent.change(slider, { target: { value: "20000000" } });
    await act(async () => fireEvent.keyUp(slider, { key: "ArrowRight" }));

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("Có người vừa thay đổi thẻ này. Tải lại để tiếp tục.");
    await user.click(within(alert).getByRole("button", { name: "Tải lại" }));
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(slider).toHaveValue("10000000");
  });

  it("shows the newest 8 journals, then loads the next page by cursor MCN-309-AC1", async () => {
    const all = Array.from({ length: 12 }, (_, i) => ({
      journalId: String(100 - i),
      occurredAt: "2026-09-25T07:00:00Z",
      description: `Mua hàng · Cửa hàng ${i + 1}`,
      entryType: "PURCHASE" as const,
      rrn: null,
      postings: [],
    }));
    const requested: string[] = [];
    server.use(
      http.get("*/v1/cards/:cardRef/ledger", ({ request }) => {
        const url = new URL(request.url);
        requested.push(url.search);
        const limit = Number(url.searchParams.get("limit"));
        const cursor = url.searchParams.get("cursor");
        const start = cursor === null ? 0 : all.findIndex((e) => e.journalId === cursor) + 1;
        const items = all.slice(start, start + limit);
        return HttpResponse.json({ items, nextCursor: start + limit < all.length ? items.at(-1)?.journalId : null });
      }),
    );
    const user = userEvent.setup();
    await openCard("crd_normal0001");
    const ledger = region("Lịch sử tiền vào, tiền ra");

    expect(await within(ledger).findAllByRole("row")).toHaveLength(9); // header + 8
    await user.click(within(ledger).getByRole("button", { name: "Xem thêm" }));
    expect(await within(ledger).findByText("Mua hàng · Cửa hàng 12")).toBeInTheDocument();
    expect(within(ledger).getAllByRole("row")).toHaveLength(13);
    expect(within(ledger).queryByRole("button", { name: "Xem thêm" })).not.toBeInTheDocument();
    expect(requested).toEqual(["?limit=8", "?limit=8&cursor=93"]);
  });

  it("offers no load-more when the ledger has no next cursor MCN-309-AC1", async () => {
    await openCard("crd_normal0001");
    const ledger = region("Lịch sử tiền vào, tiền ra");
    expect(await within(ledger).findAllByRole("row")).toHaveLength(6);
    expect(within(ledger).queryByRole("button")).not.toBeInTheDocument();
  });

  it("labels load-more in Vietnamese in Expert mode too CARDS-G15", async () => {
    useDisplayMode.setState({ mode: "expert" });
    server.use(
      http.get("*/v1/cards/:cardRef/ledger", () =>
        HttpResponse.json({ items: [], nextCursor: "1" }),
      ),
    );
    await openCard("crd_normal0001");
    expect(await within(region("Bút toán kép (journal)")).findByRole("button", { name: "Tải thêm" })).toBeInTheDocument();
  });

  it("names the entry type when the issuer only generated a description", async () => {
    server.use(
      http.get("*/v1/cards/:cardRef/ledger", () =>
        HttpResponse.json({
          items: [
            {
              journalId: "244",
              occurredAt: "2026-09-25T07:58:03Z",
              description: "PURCHASE journal 244",
              entryType: "PURCHASE",
              rrn: "626807000352",
              postings: [
                { account: "ACC-crd_normal0001", direction: "DEBIT", amount: { amount: 10_000, currency: "704" } },
                { account: "SETTLEMENT_SUSPENSE", direction: "CREDIT", amount: { amount: 10_000, currency: "704" } },
              ],
            },
          ],
          nextCursor: null,
        }),
      ),
    );
    await openCard("crd_normal0001");
    const rows = await within(region("Lịch sử tiền vào, tiền ra")).findAllByRole("row");
    expect(rows[1]).toHaveTextContent("Mua hàng");
    expect(rows[1]).not.toHaveTextContent("journal 244");
  });

  it("shows the card's audit timeline from the issuer, newest first, with who did it CARDS-AUDIT", async () => {
    server.use(
      http.get("*/v1/cards/:cardRef/audit", () =>
        HttpResponse.json({
          items: [
            { auditId: "a2", occurredAt: "2026-09-25T07:05:00Z", actor: "ops-lan", action: "CARD_LIMITS_UPDATED", before: null, after: null },
            { auditId: "a1", occurredAt: "2026-09-25T06:30:00Z", actor: "ops-minh", action: "CARD_UNBLOCKED", before: null, after: null },
          ],
          nextCursor: null,
        }),
      ),
    );
    await openCard("crd_normal0001");
    const audit = await within(region("Trạng thái thẻ")).findByRole("list", { name: "Nhật ký thao tác" });

    const items = await within(audit).findAllByRole("listitem");
    expect(items).toHaveLength(2);
    expect(items[0]).toHaveTextContent(/^Đổi hạn mức thẻ •••• 4417 lúc \d{2}:\d{2} · ops-lan$/);
    expect(items[1]).toHaveTextContent(/^Mở khóa thẻ •••• 4417 lúc \d{2}:\d{2} · ops-minh$/);
  });
});

import { act, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";
import { renderWithIntl } from "@/test/render";
import { useDisplayMode } from "@/shared/state/display-mode";
import { useMessageLab } from "@/shared/state/message-lab";
import { FieldTable } from "./FieldTable";

const decoded = {
  mti: "0800",
  primaryBitmap: "0".repeat(16),
  secondaryBitmap: null,
  segments: [],
  fields: [{ de: "11", easyName: "Sequence number", technicalName: "STAN", format: "n", value: "000200" }],
};

describe("FieldTable", () => {
  beforeEach(() => {
    act(() => useDisplayMode.setState({ mode: "easy" }));
    act(() => useMessageLab.setState({ decoded, selectedKey: null, raw: "", setRaw: () => {}, setDecoded: () => {}, select: useMessageLab.getState().select }));
  });

  it("shows easy names and hides the format column in Easy mode __MCN_104_AC4", () => {
    renderWithIntl(<FieldTable />);
    expect(screen.getByText("Sequence number")).toBeInTheDocument();
    expect(screen.queryByText("STAN")).not.toBeInTheDocument();
    // 3 columns (DE, name, value); renderWithIntl uses the vi locale, so match on
    // count rather than an English "format" name.
    expect(screen.getAllByRole("columnheader")).toHaveLength(3);
  });

  it("shows technical names and the format column in Expert mode __MCN_104_AC4", () => {
    act(() => useDisplayMode.setState({ mode: "expert" }));
    renderWithIntl(<FieldTable />);
    expect(screen.getByText("STAN")).toBeInTheDocument();
    expect(screen.getAllByRole("columnheader")).toHaveLength(4);
  });

  it("selects a field's row on click __MCN_104_AC1", async () => {
    renderWithIntl(<FieldTable />);
    await userEvent.click(screen.getByText("000200"));
    expect(useMessageLab.getState().selectedKey).toBe("11");
  });
});

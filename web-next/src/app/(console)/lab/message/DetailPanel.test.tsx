import { act, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import { renderWithIntl } from "@/test/render";
import { useMessageLab } from "@/shared/state/message-lab";
import { DetailPanel } from "./DetailPanel";

describe("DetailPanel", () => {
  beforeEach(() =>
    act(() =>
      useMessageLab.setState({
        decoded: { mti: "0200", primaryBitmap: "0".repeat(16), secondaryBitmap: null, segments: [], fields: [{ de: "22", easyName: "Entry mode", technicalName: "POS entry mode", format: "n", value: "051" }] },
        selectedKey: "22",
        raw: "",
        setRaw: () => {},
        setDecoded: () => {},
        select: () => {},
      }),
    ),
  );

  it("explains DE 22's entry-mode code __MCN_104_AC3", () => {
    renderWithIntl(<DetailPanel />);
    expect(screen.getByText(/chip/i)).toBeInTheDocument();
  });

  it("breaks down the MTI's four digits when mti is selected", () => {
    act(() => useMessageLab.setState({ selectedKey: "mti" }));
    // renderWithIntl uses the vi locale ("Phiên bản" = "Version").
    renderWithIntl(<DetailPanel />);
    expect(screen.getByText("Phiên bản")).toBeInTheDocument();
  });
});

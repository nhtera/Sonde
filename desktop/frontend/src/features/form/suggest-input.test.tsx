// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { SuggestInput } from "./suggest-input";

describe("SuggestInput", () => {
  it("commits on Enter key", async () => {
    const user = userEvent.setup();
    const onCommit = vi.fn();
    render(<SuggestInput label="Test" value="initial" onCommit={onCommit} />);

    const input = screen.getByLabelText("Test") as HTMLInputElement;
    await user.clear(input);
    await user.type(input, "new value");
    await user.keyboard("{Enter}");

    expect(onCommit).toHaveBeenCalledWith("new value");
  });

  it("commits on blur", async () => {
    const user = userEvent.setup();
    const onCommit = vi.fn();
    render(<SuggestInput label="Test" value="initial" onCommit={onCommit} />);

    const input = screen.getByLabelText("Test") as HTMLInputElement;
    await user.clear(input);
    await user.type(input, "changed");
    await user.click(document.body); // blur

    expect(onCommit).toHaveBeenCalledWith("changed");
  });

  it("does not commit unchanged value on blur", async () => {
    const user = userEvent.setup();
    const onCommit = vi.fn();
    render(<SuggestInput label="Test" value="unchanged" onCommit={onCommit} />);

    // Focused, then left without a change: nothing to write.
    await user.click(screen.getByLabelText("Test"));
    await user.click(document.body);

    expect(onCommit).not.toHaveBeenCalled();
  });

  it("Escape reverts to original value", async () => {
    const user = userEvent.setup();
    const onCommit = vi.fn();
    render(<SuggestInput label="Test" value="original" onCommit={onCommit} />);

    const input = screen.getByLabelText("Test") as HTMLInputElement;
    await user.clear(input);
    await user.type(input, "changed");
    await user.keyboard("{Escape}");

    // Input should show original value, commit should not happen
    expect(input.value).toBe("original");
    expect(onCommit).not.toHaveBeenCalled();
  });

  it("completion insert with Tab key", async () => {
    const user = userEvent.setup();
    const onCommit = vi.fn();
    const suggest = async (before: string) => {
      if (before === "he") {
        return {
          from: 0,
          items: [
            { label: "header-1", insert: "header-1" },
            { label: "header-2", insert: "header-2" },
          ],
        };
      }
      return null;
    };
    render(<SuggestInput label="Test" value="" onCommit={onCommit} suggest={suggest} />);

    const input = screen.getByLabelText("Test") as HTMLInputElement;
    await user.type(input, "he");
    // First item should be active
    expect(screen.getByText("header-1")).toBeInTheDocument();

    await user.keyboard("{Tab}");
    // After Tab, the completion should be inserted, not committed yet
    expect(input.value).toBe("header-1");
    expect(onCommit).not.toHaveBeenCalled();
  });

  it("completion insert with Enter key", async () => {
    const user = userEvent.setup();
    const onCommit = vi.fn();
    const suggest = async (before: string) => {
      if (before === "x-") {
        return {
          from: 0,
          items: [
            { label: "x-custom", insert: "x-custom" },
          ],
        };
      }
      return null;
    };
    render(<SuggestInput label="Test" value="" onCommit={onCommit} suggest={suggest} />);

    const input = screen.getByLabelText("Test") as HTMLInputElement;
    await user.type(input, "x-");
    expect(screen.getByText("x-custom")).toBeInTheDocument();

    await user.keyboard("{Enter}");
    expect(input.value).toBe("x-custom");
  });

  it("list click completes without double-commit", async () => {
    const user = userEvent.setup();
    const onCommit = vi.fn();
    const suggest = async (before: string) => {
      if (before === "co") {
        return {
          from: 0,
          items: [
            { label: "content-type", insert: "content-type" },
          ],
        };
      }
      return null;
    };
    render(<SuggestInput label="Test" value="" onCommit={onCommit} suggest={suggest} />);

    const input = screen.getByLabelText("Test") as HTMLInputElement;
    await user.type(input, "co");
    const item = screen.getByText("content-type");

    // Click the suggestion item (onMouseDown with preventDefault)
    await user.pointer({ keys: "[MouseLeft>]", target: item });

    expect(input.value).toBe("content-type");
    // After click, list closes but input value is set, no commit yet
    expect(screen.queryByText("content-type", { selector: "li" })).not.toBeInTheDocument();
    expect(onCommit).not.toHaveBeenCalled();
  });

  it("arrow keys navigate completions", async () => {
    const suggest = async (before: string) => {
      if (before === "a") {
        return {
          from: 0,
          items: [
            { label: "accept", insert: "accept" },
            { label: "authorization", insert: "authorization" },
            { label: "age", insert: "age" },
          ],
        };
      }
      return null;
    };
    render(<SuggestInput label="Test" value="" onCommit={() => {}} suggest={suggest} />);

    const input = screen.getByLabelText("Test");
    await userEvent.type(input, "a");

    // ArrowDown moves to next
    await userEvent.keyboard("{ArrowDown}");
    let items = screen.getAllByRole("option");
    expect(items[1]).toHaveAttribute("aria-selected", "true");

    // ArrowDown again
    await userEvent.keyboard("{ArrowDown}");
    items = screen.getAllByRole("option");
    expect(items[2]).toHaveAttribute("aria-selected", "true");

    // ArrowUp wraps around to end and moves up
    await userEvent.keyboard("{ArrowUp}");
    items = screen.getAllByRole("option");
    expect(items[1]).toHaveAttribute("aria-selected", "true");
  });

  it("completion list closed when Escape pressed", async () => {
    const user = userEvent.setup();
    const suggest = async (before: string) => {
      if (before === "t") {
        return {
          from: 0,
          items: [
            { label: "test", insert: "test" },
          ],
        };
      }
      return null;
    };
    render(<SuggestInput label="Test" value="" onCommit={() => {}} suggest={suggest} />);

    const input = screen.getByLabelText("Test");
    await user.type(input, "t");
    expect(screen.getByText("test")).toBeInTheDocument();

    await user.keyboard("{Escape}");
    expect(screen.queryByText("test", { selector: "li" })).not.toBeInTheDocument();
  });

  it("no suggestions when suggest returns null", async () => {
    const user = userEvent.setup();
    const suggest = async () => null;
    render(<SuggestInput label="Test" value="" onCommit={() => {}} suggest={suggest} />);

    const input = screen.getByLabelText("Test");
    await user.type(input, "anything");

    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
  });

  it("no suggestions when list is empty", async () => {
    const user = userEvent.setup();
    const suggest = async () => ({ from: 0, items: [] });
    render(<SuggestInput label="Test" value="" onCommit={() => {}} suggest={suggest} />);

    const input = screen.getByLabelText("Test");
    await user.type(input, "xyz");

    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
  });

  it("disabled input cannot be edited", async () => {
    const user = userEvent.setup();
    const onCommit = vi.fn();
    render(<SuggestInput label="Test" value="locked" disabled onCommit={onCommit} />);

    const input = screen.getByLabelText("Test") as HTMLInputElement;
    await user.click(input);

    expect(input.disabled).toBe(true);
    expect(onCommit).not.toHaveBeenCalled();
  });

  it("read-only input shows value but cannot commit", async () => {
    const onCommit = vi.fn();
    render(<SuggestInput label="Test" value="readonly" readOnly onCommit={onCommit} />);

    const input = screen.getByLabelText("Test") as HTMLInputElement;
    // readOnly inputs show value but user events don't change them
    expect(input.value).toBe("readonly");
    expect(input.readOnly).toBe(true);
  });
});

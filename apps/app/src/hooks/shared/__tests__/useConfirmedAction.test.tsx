import { renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import useConfirmedAction from "../useConfirmedAction";

const toast = {
  success: vi.fn(),
  error: vi.fn(),
  info: vi.fn(),
  warning: vi.fn(),
};

vi.mock("../../useToast", () => ({
  default: () => toast,
}));

describe("useConfirmedAction", () => {
  beforeEach(() => {
    vi.stubGlobal("confirm", vi.fn(() => true));
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it("runs action after confirmation and shows success", async () => {
    const onSuccess = vi.fn();
    const { result } = renderHook(() => useConfirmedAction());

    const response = await result.current({
      confirmMessage: "confirm?",
      request: async () => "ok",
      successMessage: "done",
      onSuccess,
    });

    expect(confirm).toHaveBeenCalledWith("confirm?");
    expect(response).toEqual({ ok: true, cancelled: false, result: "ok" });
    expect(toast.success).toHaveBeenCalledWith("done");
    expect(onSuccess).toHaveBeenCalledWith("ok");
  });

  it("aborts when user cancels confirmation", async () => {
    vi.stubGlobal("confirm", vi.fn(() => false));
    const request = vi.fn();
    const { result } = renderHook(() => useConfirmedAction());

    const response = await result.current({
      confirmMessage: "confirm?",
      request,
    });

    expect(request).not.toHaveBeenCalled();
    expect(response).toEqual({ ok: false, cancelled: true });
  });

  it("shows error when action fails", async () => {
    const { result } = renderHook(() => useConfirmedAction());

    const response = await result.current({
      request: async () => {
        throw new Error("boom");
      },
      errorMessage: "fallback",
    });

    expect(response.ok).toBe(false);
    expect(response.cancelled).toBe(false);
    expect(toast.error).toHaveBeenCalledWith("boom");
  });
});

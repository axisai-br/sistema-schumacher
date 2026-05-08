import { renderHook, waitFor, act } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { createTestQueryClient, withQueryClient } from "../../../test/queryTestUtils";
import { apiDelete, apiGet, apiPatch, apiPost } from "../../../services/api";
import { useEntityList } from "../useEntityList";
import { createCrudMutations } from "../createCrudMutations";
import { createEntityActions } from "../createEntityActions";

vi.mock("../../../services/api", () => ({
  apiGet: vi.fn(),
  apiPost: vi.fn(),
  apiPatch: vi.fn(),
  apiDelete: vi.fn(),
}));

describe("shared data foundation", () => {
  afterEach(() => {
    vi.clearAllMocks();
  });

  it("builds list query with filters and ignores empty values", async () => {
    vi.mocked(apiGet).mockResolvedValueOnce([{ id: "1" }] as never);

    const client = createTestQueryClient();
    const { result } = renderHook(
      () =>
        useEntityList<{ id: string }, { search?: string; status?: string; empty?: string }>({
          queryKey: ["entities"],
          path: "/entities",
          limit: 20,
          offset: 40,
          filters: { search: " test ", status: "active", empty: "" },
        }),
      { wrapper: withQueryClient(client) }
    );

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(apiGet).toHaveBeenCalledWith("/entities?limit=20&offset=40&search=test&status=active");
  });

  it("invalidates configured query keys for CRUD mutations", async () => {
    vi.mocked(apiPost).mockResolvedValue({ id: "1" } as never);
    vi.mocked(apiPatch).mockResolvedValue({ id: "1" } as never);
    vi.mocked(apiDelete).mockResolvedValue(undefined as never);

    const client = createTestQueryClient();
    const invalidateSpy = vi.spyOn(client, "invalidateQueries");

    const crud = createCrudMutations<{ name: string }, { name?: string }, { id: string }>({
      basePath: "/entities",
      queryKey: ["entities"],
      invalidateQueryKeys: [["other"]],
    });

    const { result: createResult } = renderHook(() => crud.useCreateEntity(), {
      wrapper: withQueryClient(client),
    });

    await act(async () => {
      await createResult.current.mutateAsync({ name: "test" });
    });

    expect(apiPost).toHaveBeenCalledWith("/entities", { name: "test" });
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ["entities"] });
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ["other"] });

    const { result: updateResult } = renderHook(() => crud.useUpdateEntity(), {
      wrapper: withQueryClient(client),
    });

    await act(async () => {
      await updateResult.current.mutateAsync({ id: "entity-1", data: { name: "updated" } });
    });

    expect(apiPatch).toHaveBeenCalledWith("/entities/entity-1", { name: "updated" });

    const { result: deleteResult } = renderHook(() => crud.useDeleteEntity(), {
      wrapper: withQueryClient(client),
    });

    await act(async () => {
      await deleteResult.current.mutateAsync("entity-1");
    });

    expect(apiDelete).toHaveBeenCalledWith("/entities/entity-1");
  });

  it("executes entity action and invalidates queries", async () => {
    vi.mocked(apiPost).mockResolvedValue({ ok: true } as never);

    const client = createTestQueryClient();
    const invalidateSpy = vi.spyOn(client, "invalidateQueries");

    const actions = createEntityActions({
      queryKey: ["invoices"],
      invalidateQueryKeys: [["products"]],
    });

    const { result } = renderHook(
      () =>
        actions.useEntityAction<void, { ok: boolean }>({
          buildPath: (id) => `/invoices/${id}/process`,
          method: "post",
          getBody: () => ({}),
        }),
      { wrapper: withQueryClient(client) }
    );

    await act(async () => {
      await result.current.mutateAsync({ id: "inv-1", payload: undefined });
    });

    expect(apiPost).toHaveBeenCalledWith("/invoices/inv-1/process", {});
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ["invoices"] });
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ["products"] });
  });
});

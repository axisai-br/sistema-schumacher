import { QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { createTestQueryClient } from "../../../test/queryTestUtils";
import Saldo from "../index";

vi.mock("../../../services/api", () => ({
  apiGet: vi.fn(),
  apiPost: vi.fn(),
}));

vi.mock("../../../hooks/useCurrentUser", () => ({
  useCurrentUser: vi.fn(),
}));

vi.mock("../../../hooks/useToast", () => ({
  default: () => ({
    success: vi.fn(),
    error: vi.fn(),
  }),
}));

import { apiGet, apiPost } from "../../../services/api";
import { useCurrentUser } from "../../../hooks/useCurrentUser";

function renderPage() {
  const client = createTestQueryClient();
  return render(
    <QueryClientProvider client={client}>
      <Saldo />
    </QueryClientProvider>
  );
}

describe("Saldo page", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("bloqueia saque quando saldo disponivel e zero", async () => {
    vi.mocked(useCurrentUser).mockReturnValue({
      data: { can_access_saldo: true, has_recipient: true },
      isLoading: false,
    } as any);

    vi.mocked(apiGet).mockImplementation(async (path: string) => {
      if (path.startsWith("/affiliate/balance")) {
        return {
          available_amount: 0,
          waiting_funds_amount: 2500,
          transferred_amount: 1000,
          currency: "BRL",
          can_withdraw: false,
          withdraw_block_reason: "Sem saldo disponivel para saque",
          anticipation_info: {
            enabled: false,
            type: "full",
            delay: 365,
            volume_percentage: 0,
          },
        };
      }
      return { items: [], pagination: { limit: 20, offset: 0, total_estimate: 0 } };
    });

    renderPage();

    expect(await screen.findByText("Saldo disponivel")).toBeInTheDocument();
    expect(screen.getByText("Informativos de antecipacao")).toBeInTheDocument();
    const withdrawButton = screen.getByRole("button", { name: "Sacar" });
    expect(withdrawButton).toBeDisabled();
    expect(screen.getByText(/Sem saldo disponivel para saque/i)).toBeInTheDocument();
  });

  it("envia saque em centavos e atualiza estados", async () => {
    const user = userEvent.setup();
    vi.mocked(useCurrentUser).mockReturnValue({
      data: { can_access_saldo: true, has_recipient: true },
      isLoading: false,
    } as any);

    vi.mocked(apiGet).mockImplementation(async (path: string) => {
      if (path.startsWith("/affiliate/balance")) {
        return {
          available_amount: 12000,
          waiting_funds_amount: 0,
          transferred_amount: 0,
          currency: "BRL",
          can_withdraw: true,
          anticipation_info: {
            enabled: true,
            type: "full",
            delay: 30,
            volume_percentage: 100,
          },
        };
      }
      return { items: [], pagination: { limit: 20, offset: 0, total_estimate: 0 } };
    });

    vi.mocked(apiPost).mockResolvedValue({
      withdrawal_id: "wdr_1",
      transfer_id: "tr_1",
      status: "pending",
      message: "Saque solicitado com sucesso",
      requested_amount: 1050,
      currency: "BRL",
    });

    renderPage();

    await screen.findByText("Saldo disponivel");
    await user.click(screen.getByRole("button", { name: "Sacar" }));
    await user.type(screen.getByRole("spinbutton"), "10.5");
    await user.click(screen.getByRole("button", { name: "Confirmar saque" }));

    await waitFor(() => {
      expect(apiPost).toHaveBeenCalledWith("/affiliate/withdraw", { amount: 1050 });
    });
  });
});

import { useMemo, useState, type FormEvent } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import PageHeader from "../../components/PageHeader";
import InlineAlert from "../../components/InlineAlert";
import LoadingState from "../../components/LoadingState";
import EmptyState from "../../components/EmptyState";
import DataTable, { type DataTableColumn } from "../../components/table/DataTable";
import Drawer from "../../components/overlay/Drawer";
import FormField from "../../components/FormField";
import PaginationControls from "../../components/input/PaginationControls";
import useToast from "../../hooks/useToast";
import { useCurrentUser } from "../../hooks/useCurrentUser";
import { apiGet, apiPost } from "../../services/api";
import type {
  AffiliateBalance,
  AffiliateWithdrawResponse,
  AffiliateWithdrawalsHistory,
  AffiliateWithdrawalItem,
} from "../../types/affiliate";
import { formatCurrency, formatDateTime } from "../../utils/format";

const PAGE_SIZE = 20;

function formatCurrencyFromCents(valueInCents: number, currency = "BRL") {
  return formatCurrency((valueInCents || 0) / 100, currency);
}

function statusLabel(status: string) {
  const normalized = (status || "").toUpperCase();
  if (normalized === "SUCCESS") return "Sucesso";
  if (normalized === "FAILED") return "Falha";
  if (normalized === "PENDING") return "Pendente";
  return status || "-";
}

function anticipationTypeLabel(value?: string) {
  const normalized = (value || "").toLowerCase();
  if (normalized === "full") return "Total";
  if (normalized === "custom" || normalized === "percentage") return "Percentual";
  return value || "-";
}

export default function Saldo() {
  const toast = useToast();
  const queryClient = useQueryClient();
  const [page, setPage] = useState(0);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [amount, setAmount] = useState("");
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const currentUserQuery = useCurrentUser();
  const canAccessSaldo = currentUserQuery.data?.can_access_saldo ?? false;
  const hasRecipient = currentUserQuery.data?.has_recipient ?? false;

  const balanceQuery = useQuery({
    queryKey: ["affiliate-balance"],
    queryFn: () => apiGet<AffiliateBalance>("/affiliate/balance"),
    enabled: canAccessSaldo && hasRecipient,
  });

  const historyQuery = useQuery({
    queryKey: ["affiliate-withdrawals-history", page, PAGE_SIZE],
    queryFn: () =>
      apiGet<AffiliateWithdrawalsHistory>(
        `/affiliate/withdrawals-history?limit=${PAGE_SIZE}&offset=${page * PAGE_SIZE}`
      ),
    enabled: canAccessSaldo && hasRecipient,
  });

  const history = historyQuery.data?.items ?? [];
  const currency = balanceQuery.data?.currency ?? "BRL";
  const availableAmount = balanceQuery.data?.available_amount ?? 0;

  const columns = useMemo<DataTableColumn<AffiliateWithdrawalItem>[]>(
    () => [
      {
        label: "Valor",
        accessor: (item) => formatCurrencyFromCents(item.amount, item.currency),
      },
      {
        label: "Status",
        accessor: (item) => statusLabel(item.status),
      },
      {
        label: "Transferencia",
        accessor: (item) => item.transfer_id || "-",
      },
      {
        label: "Solicitado em",
        accessor: (item) => formatDateTime(item.requested_at),
      },
      {
        label: "Processado em",
        accessor: (item) => (item.processed_at ? formatDateTime(item.processed_at) : "-"),
      },
    ],
    []
  );

  const handleWithdraw = async (e: FormEvent) => {
    e.preventDefault();
    setSubmitError(null);

    const amountValue = Number(amount);
    if (!Number.isFinite(amountValue) || amountValue <= 0) {
      setSubmitError("Informe um valor maior que zero.");
      return;
    }

    const amountInCents = Math.round(amountValue * 100);
    if (amountInCents > availableAmount) {
      setSubmitError("O valor nao pode ser maior que o saldo disponivel.");
      return;
    }

    try {
      setSubmitting(true);
      const result = await apiPost<AffiliateWithdrawResponse>("/affiliate/withdraw", {
        amount: amountInCents,
      });
      toast.success(result.message || "Solicitacao de saque enviada.");
      setDrawerOpen(false);
      setAmount("");
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["affiliate-balance"] }),
        queryClient.invalidateQueries({ queryKey: ["affiliate-withdrawals-history"] }),
      ]);
    } catch (err: any) {
      setSubmitError(err?.message || "Nao foi possivel solicitar o saque.");
    } finally {
      setSubmitting(false);
    }
  };

  if (currentUserQuery.isLoading) {
    return (
      <section className="page">
        <LoadingState label="Carregando contexto do usuario..." />
      </section>
    );
  }

  if (!canAccessSaldo) {
    return (
      <section className="page">
        <PageHeader title="Saldo" subtitle="Visao financeira para afiliados/recebedores." />
        <InlineAlert tone="warning">
          Voce nao possui permissao ou vinculo de recebedor para acessar esta tela.
        </InlineAlert>
      </section>
    );
  }

  const loading = balanceQuery.isLoading || historyQuery.isLoading;
  const loadError =
    (balanceQuery.error as Error | undefined)?.message ||
    (historyQuery.error as Error | undefined)?.message ||
    null;

  return (
    <section className="page">
      <PageHeader
        title="Saldo"
        subtitle="Acompanhe seu saldo de recebedor e realize saques."
        primaryAction={
          <button
            className="button"
            type="button"
            onClick={() => {
              setSubmitError(null);
              setDrawerOpen(true);
            }}
            disabled={!balanceQuery.data?.can_withdraw}
          >
            Sacar
          </button>
        }
      />

      {loadError ? <InlineAlert tone="error">{loadError}</InlineAlert> : null}
      {loading ? <LoadingState label="Carregando saldo e historico..." /> : null}
      {!hasRecipient ? (
        <InlineAlert tone="warning">
          Seu usuario ainda nao possui recipient vinculado. A tela foi liberada, mas o saldo e os saques
          serao habilitados somente apos o vinculo.
        </InlineAlert>
      ) : null}
      {!loading && balanceQuery.data && !balanceQuery.data.can_withdraw ? (
        <InlineAlert tone="info">
          {balanceQuery.data.withdraw_block_reason || "Sem saldo disponivel para saque no momento."}
        </InlineAlert>
      ) : null}

      {balanceQuery.data ? (
        <>
          <div className="card-grid">
            <div className="section">
              <div className="section-title">Saldo disponivel</div>
              <div className="page-title">
                {formatCurrencyFromCents(balanceQuery.data.available_amount, balanceQuery.data.currency)}
              </div>
            </div>
            <div className="section">
              <div className="section-title">Saldo a receber</div>
              <div className="page-title">
                {formatCurrencyFromCents(balanceQuery.data.waiting_funds_amount, balanceQuery.data.currency)}
              </div>
            </div>
            <div className="section">
              <div className="section-title">Saldo ja transferido</div>
              <div className="page-title">
                {formatCurrencyFromCents(balanceQuery.data.transferred_amount, balanceQuery.data.currency)}
              </div>
            </div>
          </div>

          <div className="section" style={{ marginTop: "12px" }}>
            <div className="section-header">
              <div className="section-title">Informativos de antecipacao</div>
            </div>
            <div className="card-grid">
              <div className="section">
                <div className="section-title">Antecipacao automatica</div>
                <div className="page-title" style={{ fontSize: "1.1rem" }}>
                  {balanceQuery.data.anticipation_info?.enabled ? "Habilitada" : "Desabilitada"}
                </div>
              </div>
              <div className="section">
                <div className="section-title">Tipo de antecipacao</div>
                <div className="page-title" style={{ fontSize: "1.1rem" }}>
                  {anticipationTypeLabel(balanceQuery.data.anticipation_info?.type)}
                </div>
              </div>
              <div className="section">
                <div className="section-title">Prazo (dias)</div>
                <div className="page-title" style={{ fontSize: "1.1rem" }}>
                  {balanceQuery.data.anticipation_info?.delay ?? "-"}
                </div>
              </div>
              <div className="section">
                <div className="section-title">Percentual de volume</div>
                <div className="page-title" style={{ fontSize: "1.1rem" }}>
                  {balanceQuery.data.anticipation_info?.volume_percentage ?? 0}%
                </div>
              </div>
            </div>
          </div>
        </>
      ) : null}

      <div className="section">
        <div className="section-header">
          <div className="section-title">Historico de saques</div>
        </div>
        <PaginationControls
          page={page}
          pageSize={PAGE_SIZE}
          itemCount={history.length}
          onPageChange={setPage}
          disabled={historyQuery.isFetching}
        />
        <DataTable
          columns={columns}
          rows={history}
          rowKey={(item) => item.id}
          emptyState={
            <EmptyState
              title="Nenhum saque encontrado"
              description="Quando houver solicitacoes de saque, elas aparecerao aqui."
            />
          }
        />
      </div>

      <Drawer
        open={drawerOpen}
        title="Solicitar saque"
        description={`Saldo disponivel: ${formatCurrencyFromCents(availableAmount, currency)}`}
        onClose={() => setDrawerOpen(false)}
        footer={
          <>
            <button className="button secondary" type="button" onClick={() => setDrawerOpen(false)}>
              Cancelar
            </button>
            <button className="button" type="submit" form="withdraw-form" disabled={submitting}>
              {submitting ? "Solicitando..." : "Confirmar saque"}
            </button>
          </>
        }
      >
        <form id="withdraw-form" className="form-grid" onSubmit={handleWithdraw}>
          <FormField label="Valor do saque (R$)" required>
            <input
              className="input"
              type="number"
              min={0}
              step={0.01}
              value={amount}
              onChange={(event) => setAmount(event.target.value)}
              required
            />
          </FormField>
          {submitError ? (
            <div className="full-span">
              <InlineAlert tone="error">{submitError}</InlineAlert>
            </div>
          ) : null}
        </form>
      </Drawer>
    </section>
  );
}

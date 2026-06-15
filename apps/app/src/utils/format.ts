export function formatCurrency(value: number, currency = "BRL") {
  return new Intl.NumberFormat("pt-BR", {
    style: "currency",
    currency,
    minimumFractionDigits: 2,
  }).format(value);
}

export function formatDateTime(value: string | number | Date) {
  const date = value instanceof Date ? value : new Date(value);
  return date.toLocaleString("pt-BR");
}

export function formatDate(value: string | number | Date) {
  const date = value instanceof Date ? value : new Date(value);
  return date.toLocaleDateString("pt-BR");
}

export function formatBirthDate(value?: string | null) {
  const raw = String(value ?? "").trim();
  if (!raw) return "-";

  const iso = raw.match(/^(\d{4})-(\d{2})-(\d{2})$/);
  if (iso) return `${iso[3]}-${iso[2]}-${iso[1]}`;

  const br = raw.match(/^(\d{2})[/-](\d{2})[/-](\d{4})$/);
  if (br) return `${br[1]}-${br[2]}-${br[3]}`;

  return raw;
}

export function formatShortId(id: string, size = 8) {
  if (!id) return "-";
  return id.slice(0, size);
}

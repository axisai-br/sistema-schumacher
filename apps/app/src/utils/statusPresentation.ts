export type StatusTone = "neutral" | "info" | "success" | "warning" | "danger";

export type StatusPresentation = {
  label: string;
  tone: StatusTone;
};

export function createStatusPresenter(
  labels: Record<string, string>,
  tones: Record<string, StatusTone>,
  fallbackTone: StatusTone = "neutral"
) {
  return (status: string): StatusPresentation => ({
    label: labels[status] ?? status,
    tone: tones[status] ?? fallbackTone,
  });
}

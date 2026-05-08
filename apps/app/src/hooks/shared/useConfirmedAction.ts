import { APIRequestError } from "../../services/api";
import useToast from "../useToast";

type ConfirmedActionOptions<T> = {
  confirmMessage?: string;
  request: () => Promise<T>;
  successMessage?: string;
  errorMessage?: string;
  onSuccess?: (result: T) => void | Promise<void>;
  onError?: (error: unknown) => void | Promise<void>;
};

type ConfirmedActionResult<T> =
  | { ok: true; cancelled: false; result: T }
  | { ok: false; cancelled: true }
  | { ok: false; cancelled: false; error: unknown };

function toErrorMessage(error: unknown, fallback = "Erro ao executar a acao") {
  if (error instanceof APIRequestError) return error.message;
  if (error instanceof Error) return error.message;
  return fallback;
}

export default function useConfirmedAction() {
  const toast = useToast();

  return async function runConfirmedAction<T>(
    options: ConfirmedActionOptions<T>
  ): Promise<ConfirmedActionResult<T>> {
    if (options.confirmMessage && !window.confirm(options.confirmMessage)) {
      return { ok: false, cancelled: true };
    }

    try {
      const result = await options.request();
      if (options.successMessage) {
        toast.success(options.successMessage);
      }
      await options.onSuccess?.(result);
      return { ok: true, cancelled: false, result };
    } catch (error) {
      toast.error(toErrorMessage(error, options.errorMessage));
      await options.onError?.(error);
      return { ok: false, cancelled: false, error };
    }
  };
}

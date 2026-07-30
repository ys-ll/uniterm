import { ref, type Ref } from "vue";
import { msg } from "../services/message";

/**
 * Wraps an async store action so loading / error / surface-toast handling
 * is consistent across stores (ARCH-035). Most stores used to repeat the
 * same try/catch/console.error pattern with no user feedback; this
 * composable centralises it.
 *
 *   const { loading, error, run } = useAsyncAction(
 *     async (id: string) => { await DeleteSkill(id) },
 *     { errorMessage: 'Failed to delete skill' },
 *   )
 *   async function remove(id: string) { await run(id) }
 *
 * Options:
 *   - errorMessage: i18n key / raw string shown via msg.error on failure
 *   - successMessage: optional toast on success
 *   - rethrow: when true, the original error is rethrown after logging /
 *     toasting so callers (or tests) can still branch on it
 *   - onError: extra side-effect on error (e.g. undo optimistic update)
 */
export interface AsyncActionOptions {
  errorMessage?: string;
  successMessage?: string;
  rethrow?: boolean;
  onError?: (err: unknown) => void;
}

export interface AsyncActionHandle<TArgs extends unknown[], TResult> {
  loading: Ref<boolean>;
  error: Ref<unknown>;
  run: (...args: TArgs) => Promise<TResult | undefined>;
  reset: () => void;
}

export function useAsyncAction<TArgs extends unknown[], TResult>(
  fn: (...args: TArgs) => Promise<TResult>,
  options: AsyncActionOptions = {},
): AsyncActionHandle<TArgs, TResult> {
  const loading = ref(false);
  const error = ref<unknown>(null);

  async function run(...args: TArgs): Promise<TResult | undefined> {
    loading.value = true;
    error.value = null;
    try {
      const result = await fn(...args);
      if (options.successMessage) {
        msg.success(options.successMessage);
      }
      return result;
    } catch (e) {
      error.value = e;
      // eslint-disable-next-line no-console
      console.error(options.errorMessage ?? "Async action failed:", e);
      if (options.errorMessage) {
        msg.error(options.errorMessage);
      }
      if (options.onError) {
        options.onError(e);
      }
      if (options.rethrow) {
        throw e;
      }
      return undefined;
    } finally {
      loading.value = false;
    }
  }

  function reset() {
    loading.value = false;
    error.value = null;
  }

  return { loading, error, run, reset };
}

import { Component, type ErrorInfo, type ReactNode } from "react";

interface Props {
  /** 出错时显示的备用 UI；默认渲染红色错误面板。 */
  fallback?: (err: Error, info: ErrorInfo) => ReactNode;
  /** 出错时回调（用于上报、上 toast、main.tsx 的 #app-runtime-error 等） */
  onError?: (err: Error, info: ErrorInfo) => void;
  children: ReactNode;
}

interface State {
  err: Error | null;
  info: ErrorInfo | null;
}

/**
 * ErrorBoundary：包住每个 view，避免某个子组件抛错时主区被静默清空。
 * Vue 时代曾因 lucide-vue-next 在 prop default 上的工厂调用 bug
 * 导致整个号池主区空白，迁移 React 时引入此兜底。
 */
export class ErrorBoundary extends Component<Props, State> {
  state: State = { err: null, info: null };

  static getDerivedStateFromError(err: Error): Partial<State> {
    return { err };
  }

  componentDidCatch(err: Error, info: ErrorInfo): void {
    this.setState({ err, info });
    this.props.onError?.(err, info);
    const handler = (window as any).__showRuntimeError;
    if (typeof handler === "function") {
      handler("react:errorBoundary", err);
    } else {
      // eslint-disable-next-line no-console
      console.error("ErrorBoundary:", err, info);
    }
  }

  reset = (): void => {
    this.setState({ err: null, info: null });
  };

  render(): ReactNode {
    if (this.state.err) {
      if (this.props.fallback && this.state.info) {
        return this.props.fallback(this.state.err, this.state.info);
      }
      return (
        <div className="flex flex-col items-center justify-center flex-1 p-8 text-center text-rose-700 dark:text-rose-300">
          <div className="max-w-[640px] w-full rounded-ios-card border border-rose-500/30 bg-rose-500/[0.06] p-6">
            <div className="text-[18px] font-bold mb-3">⚠️ 这一页渲染失败了</div>
            <pre className="text-[12px] text-left overflow-auto max-h-[40vh] rounded-lg bg-black/[0.06] dark:bg-white/[0.06] p-3 font-mono">
              {String(this.state.err.stack || this.state.err.message)}
            </pre>
            <div className="mt-4 flex justify-center gap-2">
              <button
                type="button"
                onClick={this.reset}
                className="rounded-full bg-rose-500/15 hover:bg-rose-500/25 px-4 py-2 text-[13px] font-bold transition-colors"
              >
                重试
              </button>
            </div>
          </div>
        </div>
      );
    }
    return this.props.children;
  }
}

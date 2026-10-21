/** One selected channel, one bounded host wait, no overlapping stream workers. */
export interface WatchOptions {
  readonly realtime?: boolean;
  readonly pollMs?: number;
  readonly setTimer?: (callback: () => void, ms: number) => unknown;
  readonly clearTimer?: (handle: unknown) => void;
}

export class ChannelWatch {
  private channel: string | null = null;
  private generation = 0;
  private timer: unknown = null;
  private running = false;
  private readonly delay: number;
  private readonly setTimer: NonNullable<WatchOptions["setTimer"]>;
  private readonly clearTimer: NonNullable<WatchOptions["clearTimer"]>;

  constructor(
    private readonly update: (channel: string, current: () => boolean) => Promise<{ readonly realtime_woke?: boolean } | null>,
    private readonly options: WatchOptions,
  ) {
    this.delay = options.pollMs ?? 4000;
    this.setTimer = options.setTimer ?? ((callback, ms) => setTimeout(callback, ms));
    this.clearTimer = options.clearTimer ?? ((handle) => clearTimeout(handle as ReturnType<typeof setTimeout>));
  }

  select(channel: string): void {
    if (this.channel === channel || this.delay <= 0) return;
    this.dispose();
    this.channel = channel;
    if (!this.running) this.schedule(this.options.realtime ? 0 : this.delay);
  }

  dispose(): void {
    this.generation += 1;
    this.channel = null;
    if (this.timer !== null) this.clearTimer(this.timer);
    this.timer = null;
  }

  private schedule(delay: number): void {
    this.timer = this.setTimer(() => { void this.tick(); }, delay);
  }

  private async tick(): Promise<void> {
    this.timer = null;
    const channel = this.channel;
    if (channel === null) return;
    const generation = this.generation;
    this.running = true;
    try {
      const report = await this.update(channel, () => this.generation === generation);
      if (this.channel !== null) this.schedule(report?.realtime_woke ? 0 : this.delay);
    } finally {
      this.running = false;
    }
  }
}

export type StepUpRequest = {
  action: string;
  target: string;
};

export type StepUpHandler = (req: StepUpRequest) => Promise<string>;

let handler: StepUpHandler | null = null;
let queue: Promise<unknown> = Promise.resolve();

export function registerStepUpHandler(next: StepUpHandler | null): void {
  handler = next;
}

// requestStepUp shows one confirmation at a time. Two dangerous calls produce two dialogs.
export function requestStepUp(req: StepUpRequest): Promise<string> {
  const run = queue.then(() => {
    if (!handler) {
      throw new Error("step-up handler is not registered");
    }
    return handler(req);
  });
  queue = run.then(
    () => undefined,
    () => undefined,
  );
  return run;
}

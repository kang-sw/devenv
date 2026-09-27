/** Pi-owned global rules. Provider and model ID are separate: IDs may contain '/'. */
export interface ModelPromptSelector {
  id: string;
  provider?: string;
}

export type ModelPromptChannel = "general" | "leadOnly";

export interface ModelPromptRule extends ModelPromptSelector {
  general?: string;
  leadOnly?: string;
}

export interface ModelPromptConfig {
  version: 1;
  rules: ModelPromptRule[];
}

export interface ModelPromptStore {
  load(): Promise<ModelPromptConfig>;
  /** Re-read under a cross-process lock; undefined explicitly clears only this channel. */
  update(selector: ModelPromptSelector, channel: ModelPromptChannel, text: string | undefined): Promise<ModelPromptConfig>;
}

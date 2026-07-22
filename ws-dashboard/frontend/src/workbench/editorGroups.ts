import type { ReactNode } from "react";
import type { ViewState } from "../resourceModel.js";
import type { WorkbenchPaneCategory } from "./editorGroupModel.js";
import type { SurfaceKind } from "./surfaceRegistry.js";

export type WorkbenchPane = {
  readonly id: string;
  readonly kind: SurfaceKind;
  readonly category: WorkbenchPaneCategory;
  readonly title: string;
  readonly detail: string;
  readonly state: ViewState;
  readonly meta: readonly string[];
  readonly contentRevision?: string;
  readonly body?: ReactNode;
};

export type WorkbenchEditorGroupModel = {
  readonly id: string;
  readonly label: string;
  readonly panes: readonly WorkbenchPane[];
};

export const initialWorkbenchGroups = [
  { id: "group-1", label: "group 1" },
  { id: "group-2", label: "group 2" },
] as const;

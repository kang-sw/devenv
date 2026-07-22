import {
  Activity,
  BriefcaseBusiness,
  Eye,
  Folder,
  FolderGit2,
  FolderOpen,
  GitBranch,
  LayoutPanelTop,
  ListTodo,
  Stethoscope,
  type LucideIcon,
} from "lucide-react";
import type { WorkRootView } from "./resourceModel.js";

export type WorkbenchToggle =
  | "viewer"
  | "task"
  | "diagnostics"
  | "events"
  | "layout";

export function ChromeIconButton({
  icon: Icon,
  label,
  className = "",
  commandId,
  disabled = false,
  tone = "default",
  onClick,
}: {
  icon: LucideIcon;
  label: string;
  className?: string;
  commandId: string;
  disabled?: boolean;
  tone?: "default" | "primary" | "danger";
  onClick: () => void;
}) {
  return (
    <button
      aria-label={label}
      className={`icon-button icon-button-${tone} ${className}`.trim()}
      data-command-id={commandId}
      disabled={disabled}
      title={label}
      type="button"
      onClick={onClick}
    >
      <Icon aria-hidden="true" size={15} strokeWidth={1.8} />
    </button>
  );
}

export function ResourceGlyph({
  presentation,
}: {
  presentation: "compactWorkRoot" | "workspace" | "workRoot";
}) {
  if (presentation === "compactWorkRoot") {
    return (
      <span
        className="resource-row-icon resource-row-icon-compact"
        aria-hidden="true"
      >
        <FolderOpen size={15} strokeWidth={1.8} />
      </span>
    );
  }

  const Icon = presentation === "workspace" ? BriefcaseBusiness : FolderGit2;
  return (
    <span className="resource-row-icon" aria-hidden="true">
      <Icon size={15} strokeWidth={1.8} />
    </span>
  );
}

export function WorkRootKindIcon({ kind }: { kind: WorkRootView["kind"] }) {
  const Icon =
    kind === "plainDirectory"
      ? Folder
      : kind === "gitLinkedWorktree"
        ? GitBranch
        : FolderGit2;
  return <Icon aria-hidden="true" size={14} strokeWidth={1.8} />;
}

export function workbenchToggleIcon(toggle: WorkbenchToggle): LucideIcon {
  switch (toggle) {
    case "viewer":
      return Eye;
    case "task":
      return ListTodo;
    case "diagnostics":
      return Stethoscope;
    case "events":
      return Activity;
    case "layout":
      return LayoutPanelTop;
  }
}

export function ToggleIcon({ toggle }: { toggle: WorkbenchToggle }) {
  const Icon = workbenchToggleIcon(toggle);
  return <Icon aria-hidden="true" size={14} strokeWidth={1.8} />;
}

export function InlineNotice({
  tone,
  title,
  detail,
}: {
  tone: "error" | "info";
  title: string;
  detail: string;
}) {
  return (
    <div className={`inline-notice inline-notice-${tone}`} role="status">
      <strong>{title}</strong>
      <span>{detail}</span>
    </div>
  );
}

import type { KeyboardEvent, ReactNode, Ref } from "react";
import { cn } from "@datasafe/ui/cn";

// Header and options share one template, the way the console table shares a column.
// max-content keeps the date on one line. A fixed 11rem column wrapped the timestamp.
// 10rem keeps Access in the middle-right, the same place as the old table.
// max-content pinned that word to the right edge.
export const bucketColumns = "minmax(0,1fr) 10rem";
export const objectColumns = "minmax(0,1fr) max-content max-content auto";

// Chromium's UIA ISelectionProvider::GetSelection walks descendants and
// keeps only item-like roles (listbox option, treeitem, menuitem). A grid
// row and a gridcell are not item-like, so aria-selected on them leaves
// GetSelection empty even when IsSelected is true. A listbox option is
// item-like, so the selected option is the selection.
export function SelectionList(props: {
  label: string;
  idPrefix: string;
  activeId?: string;
  columns: ReactNode[];
  template: string;
  empty?: ReactNode;
  children: ReactNode;
}) {
  const columnIds = props.columns.map((_, index) => `${props.idPrefix}-col-${index}`);
  const span = { gridColumn: "1 / -1", gridTemplateColumns: "subgrid" } as const;
  return (
    <div className="grid w-full text-sm" style={{ gridTemplateColumns: props.template }}>
      <div className="grid items-center border-b text-left font-bold text-muted-foreground" style={span}>
        {props.columns.map((column, index) => (
          <div key={columnIds[index]} id={columnIds[index]} className="whitespace-nowrap p-3">
            {column}
          </div>
        ))}
      </div>
      {props.empty ? (
        <div style={{ gridColumn: "1 / -1" }}>{props.empty}</div>
      ) : (
        <div
          role="listbox"
          aria-label={props.label}
          aria-describedby={columnIds.join(" ")}
          aria-multiselectable="false"
          aria-activedescendant={props.activeId}
          className="grid"
          style={span}
        >
          {props.children}
        </div>
      )}
    </div>
  );
}

export function SelectionOption(props: {
  id: string;
  selected: boolean;
  tabIndex: number;
  labelledBy?: string;
  describedBy?: string;
  keyShortcuts?: string;
  optionRef?: Ref<HTMLDivElement>;
  className?: string;
  onClick?: () => void;
  onKeyDown?: (event: KeyboardEvent<HTMLDivElement>) => void;
  children: ReactNode;
}) {
  return (
    <div
      id={props.id}
      role="option"
      aria-selected={props.selected ? "true" : "false"}
      aria-labelledby={props.labelledBy}
      aria-describedby={props.describedBy}
      aria-keyshortcuts={props.keyShortcuts}
      tabIndex={props.tabIndex}
      ref={props.optionRef}
      onClick={props.onClick}
      onKeyDown={props.onKeyDown}
      className={cn(
        "grid cursor-pointer items-center border-b hover:bg-muted/50 outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring",
        props.selected && "bg-muted/30",
        props.className,
      )}
      style={{ gridColumn: "1 / -1", gridTemplateColumns: "subgrid" }}
    >
      {props.children}
    </div>
  );
}

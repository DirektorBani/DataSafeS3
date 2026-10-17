import type { KeyboardEvent, ReactNode, Ref } from "react";
import { cn } from "@datasafe/ui/cn";

// The header and each option share one template so the columns line up.
export const bucketColumns = "minmax(0,1fr) 10rem";
export const objectColumns = "minmax(0,1.4fr) 8rem 11rem 3rem";

// Chromium's UIA ISelectionProvider::GetSelection walks descendants and
// keeps only item-like roles (listbox option, treeitem, menuitem). A grid
// row and a gridcell are not item-like, so aria-selected on them leaves
// GetSelection empty even when IsSelected is true. A listbox option is
// item-like, so the selected option is the selection.
export function SelectionList(props: {
  label: string;
  activeId?: string;
  columns: ReactNode[];
  template: string;
  children: ReactNode;
}) {
  return (
    <div className="w-full text-sm">
      <div className="grid border-b text-left text-muted-foreground" style={{ gridTemplateColumns: props.template }} aria-hidden="true">
        {props.columns.map((column, index) => (
          <div key={index} className="p-3">
            {column}
          </div>
        ))}
      </div>
      <div role="listbox" aria-label={props.label} aria-multiselectable="false" aria-activedescendant={props.activeId}>
        {props.children}
      </div>
    </div>
  );
}

export function SelectionOption(props: {
  id: string;
  selected: boolean;
  tabIndex: number;
  template: string;
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
      tabIndex={props.tabIndex}
      ref={props.optionRef}
      onClick={props.onClick}
      onKeyDown={props.onKeyDown}
      className={cn(
        "grid cursor-pointer border-b hover:bg-muted/50 outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring",
        props.selected && "bg-muted/30",
        props.className,
      )}
      style={{ gridTemplateColumns: props.template }}
    >
      {props.children}
    </div>
  );
}

import { SelectionList, SelectionOption, bucketColumns, objectColumns } from "./selection-list";

// Shown only when the desktop process is started with DATASAFE_UIA_FIXTURE=1.
// The Windows smoke reads Selection.GetSelection on these two lists. They use
// the same listbox widget as the bucket and object lists.
export function UiaFixture() {
  return (
    <main className="space-y-6 p-6">
      <SelectionList label="Buckets" activeId="bucket-row-0" columns={["Name", "Access"]} template={bucketColumns}>
        <SelectionOption id="bucket-row-0" selected tabIndex={0} template={bucketColumns}>
          <span className="p-3 font-medium">files</span>
          <span className="p-3 text-muted-foreground">Owned</span>
        </SelectionOption>
      </SelectionList>
      <SelectionList label="Objects" activeId="object-row-0" columns={["Name", "Size", "Modified", ""]} template={objectColumns}>
        <SelectionOption id="object-row-0" selected tabIndex={0} template={objectColumns}>
          <span className="p-3 font-mono">readme.txt</span>
          <span className="p-3">12</span>
          <span className="p-3" />
          <span className="p-3" />
        </SelectionOption>
      </SelectionList>
    </main>
  );
}

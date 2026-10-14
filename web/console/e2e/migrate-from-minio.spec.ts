import { test, expect } from "@playwright/test";
import { loginAsAdmin } from "./helpers";

test("migrate settings build a command and read evidence locally", async ({ page }) => {
  await loginAsAdmin(page);
  await page.goto("/admin/settings/migrate");
  await expect(page.getByTestId("migrate-settings")).toBeVisible({ timeout: 15_000 });
  await expect(page.locator('input[type="password"]')).toHaveCount(0);

  await page.getByTestId("migrate-source").fill("http://minio.internal:9000");
  await page.getByTestId("migrate-target").fill("http://datasafe.internal:9000");
  await page.getByTestId("migrate-buckets").fill("logs, media");
  const command = page.getByTestId("migrate-command");
  await expect(command).toContainText("--source 'http://minio.internal:9000'");
  await expect(command).toContainText("--target 'http://datasafe.internal:9000'");
  await expect(command).toContainText("--buckets 'logs,media'");
  await expect(command).toContainText('$SOURCE_ACCESS_KEY');
  await expect(command).toContainText('$SOURCE_SECRET_KEY');
  await expect(command).not.toContainText("datasafesecret");
  await expect(command).not.toContainText("minioadmin");
  const powershell = page.getByTestId("migrate-command-powershell");
  await expect(powershell).toContainText("$env:SOURCE_ACCESS_KEY");
  await expect(powershell).toContainText("$env:SOURCE_SECRET_KEY");
  await expect(powershell).toContainText("--buckets 'logs,media'");
  await expect(powershell).not.toContainText("datasafesecret");

  await page.getByTestId("migrate-evidence-file").setInputFiles({
    name: "evidence.json",
    mimeType: "application/json",
    buffer: Buffer.from(
      JSON.stringify({
        verdicts: {
          object_count: "PASS",
          bytes: "PASS",
          delete_markers: "PASS",
          etag: "PASS",
          object_lock: "PASS",
          content_sample: "FAIL",
        },
        warnings: ["policy statement DropMe not copied: StringEquals"],
        buckets: [
          {
            name: "logs",
            source_objects: 2,
            target_objects: 2,
            source_bytes: 10,
            target_bytes: 10,
            source_delete_markers: 1,
            target_delete_markers: 1,
          },
        ],
      }),
    ),
  });
  await expect(page.getByTestId("migrate-verdict-object_count")).toContainText("PASS");
  await expect(page.getByTestId("migrate-verdict-content_sample")).toContainText("FAIL");
  await expect(page.getByTestId("migrate-evidence")).toContainText("DropMe");
  await expect(page.getByTestId("migrate-bucket-counts")).toContainText("logs: 2/2 objects, 10/10 bytes, 1/1 delete markers");
});

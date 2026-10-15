import { expect, test } from "@playwright/test";
import { adminToken, createBucket, e2eSuffix } from "./helpers";

test("disabled share page shows the administrator sentence", async ({ page, request, baseURL }) => {
  const suffix = e2eSuffix();
  const bucket = `e06-share-${suffix}`;
  const token = await adminToken(request, baseURL!);
  await createBucket(request, baseURL!, token, bucket);

  const put = await request.put(
    `${baseURL}/api/v1/buckets/${encodeURIComponent(bucket)}/objects/note.txt`,
    {
      headers: { Authorization: `Bearer ${token}`, "Content-Type": "text/plain" },
      data: "hello-e06",
    }
  );
  if (!put.ok()) {
    throw new Error(`upload failed: ${put.status()} ${await put.text()}`);
  }

  const share = await request.post(`${baseURL}/api/v1/buckets/${encodeURIComponent(bucket)}/shares`, {
    headers: { Authorization: `Bearer ${token}` },
    data: { key: "note.txt", expires_in_sec: 3600 },
  });
  if (!share.ok()) {
    throw new Error(`share failed: ${share.status()} ${await share.text()}`);
  }
  const created = (await share.json()) as { share?: { token?: string } };
  const shareToken = created.share?.token;
  expect(shareToken).toBeTruthy();

  const off = await request.put(`${baseURL}/api/v1/buckets/${encodeURIComponent(bucket)}/sharing`, {
    headers: { Authorization: `Bearer ${token}` },
    data: { share_links_allowed: false },
  });
  if (!off.ok()) {
    throw new Error(`disable sharing failed: ${off.status()} ${await off.text()}`);
  }

  await page.goto(`/share/${shareToken}`);
  await expect(page.getByText("File sharing for this bucket is disabled by an administrator")).toBeVisible();
  await expect(page.getByText("Ask the bucket administrator to turn sharing back on.")).toBeVisible();
  await expect(page.getByText("sharing_disabled")).toHaveCount(0);
});

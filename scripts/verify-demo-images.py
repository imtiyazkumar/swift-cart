"""Check the direct public seed-image URLs, without credentials or an API key."""
import concurrent.futures
import json
from pathlib import Path
import urllib.request

manifest_path = Path(__file__).resolve().parents[1] / "internal/demoseed/demo_images.json"
photos = json.loads(manifest_path.read_text())
def check(photo):
    request = urllib.request.Request(photo["url"], headers={"User-Agent": "Yember-local-fixture-check/1.0"})
    with urllib.request.urlopen(request, timeout=20) as response:
        kind = response.headers.get("Content-Type", "")
        if response.status != 200 or not kind.startswith("image/"):
            raise RuntimeError(f"{photo['name']}: invalid image response")
        # Read actual image bytes, rather than accepting an HTML landing page.
        content = response.read(64)
        if not content.startswith((b"\xff\xd8\xff", b"\x89PNG", b"RIFF")):
            raise RuntimeError(f"{photo['name']}: unexpected image signature")
        return f"{photo['name']}: HTTP 200, {kind}"
with concurrent.futures.ThreadPoolExecutor(max_workers=5) as executor:
    for result in executor.map(check, photos):
        print(result)
print(f"Verified {len(photos)} public image URLs.")

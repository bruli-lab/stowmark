const GITHUB_OWNER = "bruli-lab";

const releaseSummary = document.querySelector("#release-summary");

const projects = [
  {
    repository: "stowmark",
    summary: document.querySelector("#stowmark-release-summary"),
    grid: document.querySelector("#stowmark-download-grid"),
    error: document.querySelector("#stowmark-release-error")
  },
  {
    repository: "stowmark-console",
    summary: document.querySelector("#console-release-summary"),
    grid: document.querySelector("#console-download-grid"),
    error: document.querySelector("#console-release-error")
  }
];

function formatBytes(bytes) {
  if (!Number.isFinite(bytes) || bytes <= 0) return "";
  const units = ["B", "KB", "MB", "GB"];
  const index = Math.min(
    Math.floor(Math.log(bytes) / Math.log(1024)),
    units.length - 1
  );

  return `${(bytes / Math.pow(1024, index)).toFixed(index === 0 ? 0 : 1)} ${units[index]}`;
}

function assetKind(filename) {
  const name = filename.toLowerCase();

  if (name.endsWith(".deb")) return "Debian / Ubuntu package";
  if (name.endsWith(".rpm")) return "RPM package";
  if (name.endsWith(".pkg")) return "macOS installer";
  if (name.endsWith(".zip")) return "ZIP archive";
  if (name.endsWith(".tar.gz") || name.endsWith(".tgz")) return "Compressed archive";
  if (name.includes("checksum")) return "Checksums";

  return "Release asset";
}

function createDownloadCard(asset) {
  const article = document.createElement("article");
  article.className = "download-card";

  const header = document.createElement("div");
  header.className = "download-card-header";

  const title = document.createElement("h3");
  title.textContent = asset.name;

  const size = document.createElement("span");
  size.className = "asset-size";
  size.textContent = formatBytes(asset.size);

  const kind = document.createElement("p");
  kind.className = "asset-kind";
  kind.textContent = assetKind(asset.name);

  const link = document.createElement("a");
  link.className = "download-link";
  link.href = asset.browser_download_url;
  link.textContent = "Download";
  link.setAttribute("download", "");
  link.rel = "noreferrer";

  header.append(title, size);
  article.append(header, kind, link);

  return article;
}

async function loadLatestRelease(project) {
  const endpoint =
    `https://api.github.com/repos/${GITHUB_OWNER}/${project.repository}/releases/latest`;

  try {
    const response = await fetch(endpoint, {
      headers: { Accept: "application/vnd.github+json" }
    });

    if (!response.ok) {
      throw new Error(`GitHub API returned ${response.status}`);
    }

    const release = await response.json();
    const assets = Array.isArray(release.assets) ? release.assets : [];

    const publishedDate = release.published_at
      ? new Intl.DateTimeFormat("en", {
          year: "numeric",
          month: "short",
          day: "numeric"
        }).format(new Date(release.published_at))
      : null;

    project.summary.textContent = publishedDate
      ? `Latest release ${release.tag_name} · ${publishedDate}`
      : `Latest release ${release.tag_name}`;

    if (project.repository === "stowmark") {
      releaseSummary.textContent = project.summary.textContent;
    }

    project.grid.replaceChildren();

    if (assets.length === 0) {
      const empty = document.createElement("div");
      empty.className = "download-placeholder";
      empty.innerHTML =
        `No downloadable files are attached to this release. ` +
        `<a href="${release.html_url}">Open the release on GitHub</a>.`;
      project.grid.append(empty);
      return;
    }

    const visibleAssets = assets
      .filter((asset) => !asset.name.toLowerCase().endsWith(".sig"))
      .sort((a, b) => a.name.localeCompare(b.name));

    visibleAssets.forEach((asset) => {
      project.grid.append(createDownloadCard(asset));
    });
  } catch (error) {
    console.error(error);
    project.summary.textContent = "Latest release available on GitHub";
    project.grid.hidden = true;
    project.error.hidden = false;

    if (project.repository === "stowmark") {
      releaseSummary.textContent = "Latest release available on GitHub";
    }
  }
}

document.querySelectorAll("[data-copy-target]").forEach((button) => {
  button.addEventListener("click", async () => {
    const target = document.getElementById(button.dataset.copyTarget);
    if (!target) return;

    try {
      await navigator.clipboard.writeText(target.textContent.trim());
      const previousText = button.textContent;
      button.textContent = "Copied";
      window.setTimeout(() => {
        button.textContent = previousText;
      }, 1400);
    } catch {
      button.textContent = "Select and copy";
    }
  });
});

projects.forEach(loadLatestRelease);

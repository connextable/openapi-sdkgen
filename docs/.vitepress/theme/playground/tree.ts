export interface ArtifactLike {
  readonly content: string;
  readonly path: string;
}

export interface TreeNode {
  readonly name: string;
  readonly path: string;
  readonly type: "directory" | "file";
  readonly children?: TreeNode[];
}

export function buildArtifactTree(files: readonly ArtifactLike[]): TreeNode[] {
  const root: TreeNode[] = [];
  for (const file of files) {
    let level = root;
    const segments = file.path.split("/");
    segments.forEach((name, index) => {
      const path = segments.slice(0, index + 1).join("/");
      const type = index === segments.length - 1 ? "file" : "directory";
      let node = level.find((item) => item.name === name && item.type === type);
      if (node === undefined) {
        node = type === "directory" ? { name, path, type, children: [] } : { name, path, type };
        level.push(node);
      }
      if (node.children !== undefined) level = node.children;
    });
  }
  sortTree(root);
  return root;
}

export function findArtifact<Artifact extends ArtifactLike>(
  files: readonly Artifact[],
  selectedPath: string,
): Artifact | undefined {
  return files.find((item) => item.path === selectedPath);
}

function sortTree(nodes: TreeNode[]) {
  nodes.sort((left, right) => {
    if (left.type !== right.type) return left.type === "directory" ? -1 : 1;
    if (left.name < right.name) return -1;
    if (left.name > right.name) return 1;
    return 0;
  });
  nodes.forEach((node) => {
    if (node.children !== undefined) sortTree(node.children);
  });
}

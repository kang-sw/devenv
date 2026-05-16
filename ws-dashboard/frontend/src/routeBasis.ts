export const serverRoutePrefix = "/servers";

type RouteLocation = {
  pathname: string;
  search: string;
  hash: string;
};

export function normalizeServerRouteLocation(
  location: RouteLocation,
  serverId: string,
): string | null {
  const targetPath = serverRoutePath(serverId);
  const currentPath = location.pathname;

  if (currentPath === targetPath || currentPath.startsWith(`${targetPath}/`)) {
    return null;
  }

  if (
    currentPath === "/" ||
    currentPath === serverRoutePrefix ||
    currentPath.startsWith(`${serverRoutePrefix}/`)
  ) {
    return `${targetPath}${location.search}${location.hash}`;
  }

  return null;
}

export function serverRoutePath(serverId: string) {
  return `${serverRoutePrefix}/${encodeURIComponent(serverId)}`;
}

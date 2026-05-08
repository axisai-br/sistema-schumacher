export function isNearBottom(
  scrollTop: number,
  clientHeight: number,
  scrollHeight: number,
  threshold = 72
) {
  return scrollHeight - (scrollTop + clientHeight) <= threshold;
}

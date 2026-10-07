export function shouldReloadDocumentsForTagChange(
  nextTagIds: string[],
  previousTagIds: string[] | undefined,
  resettingForKnowledgeBase: boolean,
) {
  if (resettingForKnowledgeBase || previousTagIds === undefined) return false;
  if (nextTagIds.length !== previousTagIds.length) return true;
  return nextTagIds.some((tagId, index) => tagId !== previousTagIds[index]);
}

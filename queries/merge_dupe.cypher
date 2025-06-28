// duplicate groups
CALL {
  MATCH (n)
  WHERE n.norm IS NOT NULL
  WITH n.norm AS normVal, collect(n) AS group
  WHERE size(group) > 1
  RETURN group
}

//  split nd merge
UNWIND group AS g
WITH group[0] AS keep, group[1..] AS dups
UNWIND dups AS dup
CALL apoc.refactor.mergeNodes([keep, dup], {
  properties: "discard",
  mergeRels: true
}) YIELD node
RETURN DISTINCT " Merged" AS status, count(*) AS mergedCount

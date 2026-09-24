#!/usr/bin/env bash
# Stop-Hook: haelt die Sitzung an, solange der PR zum aktuellen Branch
# unaufgeloeste Review-Threads traegt — auch die von Copilot.
#
# Warum ein Stop-Hook und nicht PostToolUse auf "gh pr create": das
# Copilot-Review erscheint erst Minuten nach dem PR. Ein Hook direkt nach dem
# Anlegen sieht garantiert null Kommentare und meldet faelschlich "sauber".
set -uo pipefail

payload=$(cat)

# Ohne diesen Abbruch blockiert der Hook seine eigene Nacharbeit endlos: die
# Antwort auf einen Kommentar endet wieder in einem Stop.
if [ "$(printf '%s' "$payload" | jq -r '.stop_hook_active // false')" = "true" ]; then
  exit 0
fi

command -v gh >/dev/null 2>&1 || exit 0
git rev-parse --git-dir >/dev/null 2>&1 || exit 0

branch=$(git rev-parse --abbrev-ref HEAD 2>/dev/null) || exit 0
case "$branch" in main | master | HEAD) exit 0 ;; esac

# Kein PR zum Branch: nichts zu pruefen. Das ist der haeufige Fall und muss
# billig sein.
pr=$(gh pr view --json number --jq .number 2>/dev/null) || exit 0
[ -n "$pr" ] || exit 0

# --paginate: reviewThreads(first:100) allein sieht nur die erste Seite, und
# ein offener Thread dahinter waere unsichtbar — genau die Sorte stiller
# Fehler, gegen die dieser Hook gebaut ist. gh blaettert ueber $endCursor
# weiter, solange hasNextPage gilt.
open=$(gh api graphql --paginate -f query='
  query($o:String!,$r:String!,$n:Int!,$endCursor:String){
    repository(owner:$o,name:$r){
      pullRequest(number:$n){
        reviewThreads(first:100, after:$endCursor){
          nodes{ isResolved comments(first:1){ nodes{ author{login} path } } }
          pageInfo{ hasNextPage endCursor }
        }
      }
    }
  }' \
  -f o="$(gh repo view --json owner --jq .owner.login 2>/dev/null)" \
  -f r="$(gh repo view --json name --jq .name 2>/dev/null)" \
  -F n="$pr" \
  --jq '.data.repository.pullRequest.reviewThreads.nodes[]
         | select(.isResolved | not)
         | "  - \(.comments.nodes[0].author.login): \(.comments.nodes[0].path)"' 2>/dev/null) || exit 0

if [ -n "$open" ] && [ "$open" != "null" ]; then
  jq -n --arg pr "$pr" --arg open "$open" '{
    decision: "block",
    reason: ("PR #\($pr) hat unaufgeloeste Review-Threads:\n\($open)\n\n" +
             "Jeden davon beheben oder mit technischer Begruendung ablehnen, " +
             "im Thread antworten (gh api repos/{owner}/{repo}/pulls/\($pr)/comments/{id}/replies) " +
             "und den Thread dann aufloesen (GraphQL resolveReviewThread). " +
             "Erst danach gilt die Arbeit als fertig.")
  }'
  exit 0
fi

# Zweite Luecke, real geworden an PR #66: Copilot nennt Befunde auch in der
# Review-ZUSAMMENFASSUNG, ganz ohne Kommentar-Thread ("…and a footer contrast
# issue"). Die Thread-Pruefung oben sieht davon nichts, und der PR wurde mit
# zwei unbearbeiteten Befunden gemergt.
#
# Ein Review-Body laesst sich nicht "aufloesen", also wird er quittiert: die
# Antwort im PR traegt einen Marker <!-- acked-review: ID -->. Damit ist die
# Quittung genau das, was sie sein soll — die sichtbare Begruendung im PR,
# kein Nebenregister.
#
# Eine frueher hier verwendete Zeit-Heuristik ("Review aelter als der letzte
# Commit gilt als bearbeitet") ist an PR #67 durchgefallen: der Commit danach
# war ein Lint-Fix und hatte mit dem Befund nichts zu tun.
acked=$(
  {
    gh api "repos/{owner}/{repo}/issues/$pr/comments" --paginate --jq '.[].body' 2>/dev/null
    gh api "repos/{owner}/{repo}/pulls/$pr/comments" --paginate --jq '.[].body' 2>/dev/null
  } | grep -oE 'acked-review:[[:space:]]*[0-9]+' | grep -oE '[0-9]+' | sort -u
)

pending=$(gh api "repos/{owner}/{repo}/pulls/$pr/reviews" --paginate \
  --jq '.[]
        | select(.state != "APPROVED")
        | select(.body | test("Changes recommended|Needs a closer look"))
        | "\(.id)\t\(.user.login)\t\(.body | capture("### [^\n]*\n+(?<head>[^\n]+)").head // "siehe Zusammenfassung")"' \
  2>/dev/null) || exit 0

open_reviews=""
while IFS=$'\t' read -r id who head; do
  [ -n "$id" ] || continue
  printf '%s\n' "$acked" | grep -qx "$id" && continue
  open_reviews="${open_reviews}  - [$id] $who: $head"$'\n'
done <<< "$pending"

[ -n "$open_reviews" ] || exit 0

jq -n --arg pr "$pr" --arg open "$open_reviews" '{
  decision: "block",
  reason: ("PR #\($pr) hat unquittierte Review-Zusammenfassungen:\n\($open)\n" +
           "Diese Befunde stehen NUR in der Zusammenfassung, nicht als Thread. " +
           "Jeden beheben oder begruendet ablehnen, die Antwort als Kommentar in " +
           "den PR schreiben und dort den Marker <!-- acked-review: ID --> mit der " +
           "Review-ID aus der Klammer setzen. Ein Commit allein quittiert nichts.")
}'

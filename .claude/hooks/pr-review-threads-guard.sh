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

open=$(gh api graphql -f query='
  query($o:String!,$r:String!,$n:Int!){
    repository(owner:$o,name:$r){
      pullRequest(number:$n){
        reviewThreads(first:100){
          nodes{ isResolved comments(first:1){ nodes{ author{login} path } } }
        }
      }
    }
  }' \
  -f o="$(gh repo view --json owner --jq .owner.login 2>/dev/null)" \
  -f r="$(gh repo view --json name --jq .name 2>/dev/null)" \
  -F n="$pr" \
  --jq '[.data.repository.pullRequest.reviewThreads.nodes[]
         | select(.isResolved | not)
         | "  - \(.comments.nodes[0].author.login): \(.comments.nodes[0].path)"]
        | join("\n")' 2>/dev/null) || exit 0

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
# Ein Review-Body laesst sich nicht "aufloesen". Massstab ist deshalb die Zeit:
# ein Review, das JUENGER ist als der letzte Commit des Branches, ist
# nachweislich noch nicht bearbeitet. Sobald die Nacharbeit committet ist,
# verstummt der Hook — kein Deadlock, und Copilot reviewt ohnehin neu.
last_commit=$(git log -1 --format=%cI 2>/dev/null) || exit 0

pending=$(gh api "repos/{owner}/{repo}/pulls/$pr/reviews" \
  --jq --arg since "$last_commit" '
    [ .[]
      | select(.state != "APPROVED")
      | select(.submitted_at > $since)
      | select(.body | test("Changes recommended|Needs a closer look"))
      | "  - \(.user.login) (\(.submitted_at)): \(.body | capture("### [^\n]*\n+(?<head>[^\n]+)").head // "siehe Zusammenfassung")"
    ] | join("\n")' 2>/dev/null) || exit 0

[ -n "$pending" ] && [ "$pending" != "null" ] || exit 0

jq -n --arg pr "$pr" --arg pending "$pending" '{
  decision: "block",
  reason: ("PR #\($pr) hat Review-Zusammenfassungen mit Befunden, die neuer sind " +
           "als der letzte Commit — also noch unbearbeitet:\n\($pending)\n\n" +
           "Diese Befunde stehen NUR in der Zusammenfassung, nicht als Thread " +
           "(gh api repos/{owner}/{repo}/pulls/\($pr)/reviews). Jeden beheben oder " +
           "begruendet ablehnen und die Antwort committen; danach schweigt diese Pruefung.")
}'

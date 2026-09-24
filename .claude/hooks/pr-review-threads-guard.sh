#!/usr/bin/env bash
# Stop-Hook: haelt die Sitzung an, solange der PR zum aktuellen Branch
# unaufgeloeste Review-Threads traegt — auch die von Copilot.
#
# Warum ein Stop-Hook und nicht PostToolUse auf "gh pr create": das
# Copilot-Review erscheint erst Minuten nach dem PR. Ein Hook direkt nach dem
# Anlegen sieht garantiert null Kommentare und meldet faelschlich "sauber".
set -uo pipefail

payload=$(</dev/stdin)

# Ohne diesen Abbruch blockiert der Hook seine eigene Nacharbeit endlos: die
# Antwort auf einen Kommentar endet wieder in einem Stop.
#
# Bewusst ohne jq: dieser Schutz muss auch dann greifen, wenn jq fehlt —
# sonst liefe die Meldung darueber gleich in die Endlosschleife, die er
# verhindern soll.
case "$payload" in
  *'"stop_hook_active":true'* | *'"stop_hook_active": true'*) exit 0 ;;
esac

command -v gh >/dev/null 2>&1 || exit 0
git rev-parse --git-dir >/dev/null 2>&1 || exit 0

branch=$(git rev-parse --abbrev-ref HEAD 2>/dev/null) || exit 0
case "$branch" in main | master | HEAD) exit 0 ;; esac

# Kein PR zum Branch: nichts zu pruefen. Das ist der haeufige Fall und muss
# billig sein.
# "Kein PR zum Branch" ist das einzige Ergebnis, das hier schweigen darf. Ein
# abgelaufenes Token, ein Rate Limit oder ein Netzfehler sieht sonst genauso
# aus wie ein sauberer Zustand — und waere dieselbe Fail-open-Luecke, die
# weiter unten schon geschlossen ist.
# Ohne temporaere Datei: ein mktemp, das scheitert, waere wieder ein stiller
# Ausstieg an einer Stelle, die gerade fail-closed werden sollte. stdout und
# stderr kommen zusammen, das Ergebnis wird danach auseinandergehalten.
pr_out=$(gh pr view --json number --jq .number 2>&1)
pr_rc=$?

if [ $pr_rc -ne 0 ]; then
  case "$pr_out" in
    *"no pull requests found"* | *"no open pull requests"* | *"no pull request found"*) exit 0 ;;
  esac
  # Noch vor der jq-Pruefung, also von Hand gebildet.
  printf '{"decision":"block","reason":"Der Review-Guard konnte nicht feststellen, ob zu diesem Branch ein PR gehoert: %s. Das ist keine Freigabe — Ursache beheben (gh auth status, Netz, Rate Limit) oder den Hook in .claude/settings.json abschalten."}\n' \
    "$(printf '%s' "$pr_out" | tr -d '"\\' | tr '\n\r' '  ' | cut -c1-200)"
  exit 0
fi

# gh schreibt die Nummer als einzige Zeile auf stdout; eine etwaige Warnung
# auf stderr steht davor.
pr=$(printf '%s\n' "$pr_out" | tail -n 1 | tr -dc '0-9')
[ -n "$pr" ] || exit 0

# Erst ab hier wird jq gebraucht — und erst ab hier darf sein Fehlen
# blockieren: auf main, ausserhalb eines Repos oder ohne PR bleibt der Hook
# still, auch ohne jq.
if ! command -v jq >/dev/null 2>&1; then
  printf '%s\n' '{"decision":"block","reason":"Der Review-Guard braucht jq und findet es nicht. Ohne jq kann er keine Meldung bilden und wuerde jeden offenen Review-Befund dieses PR stillschweigend durchwinken. jq installieren (brew install jq) oder den Hook in .claude/settings.json abschalten."}'
  exit 0
fi

# Ab hier steht fest, dass es einen PR gibt — und ab hier ist Schweigen eine
# Aussage ("nichts offen"), die der Hook nur treffen darf, wenn er wirklich
# nachgesehen hat. Ein abgelaufenes Token, ein Rate Limit oder ein Netzfehler
# sind kein sauberer PR. Ein Guard, der bei Stoerung durchwinkt, ist wertlos.
unverifiable() {
  jq -n --arg pr "$pr" --arg what "$1" '{
    decision: "block",
    reason: ("Die Review-Pruefung fuer PR #\($pr) konnte nicht durchgefuehrt werden: \($what). " +
             "Das ist keine Freigabe — der Hook blockiert absichtlich, statt ungeprueft " +
             "durchzuwinken. Ursache beheben (gh auth status, Netz, Rate Limit) und erneut " +
             "versuchen; oder den Hook in .claude/settings.json bewusst abschalten.")
  }'
  exit 0
}

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
         | "  - \(.comments.nodes[0].author.login): \(.comments.nodes[0].path)"' 2>/dev/null) \
  || unverifiable "die GraphQL-Abfrage der Review-Threads schlug fehl"

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
acked_raw=$(gh api "repos/{owner}/{repo}/issues/$pr/comments" --paginate --jq '.[].body' 2>/dev/null) \
  || unverifiable "die Abfrage der PR-Kommentare schlug fehl"
acked_raw2=$(gh api "repos/{owner}/{repo}/pulls/$pr/comments" --paginate --jq '.[].body' 2>/dev/null) \
  || unverifiable "die Abfrage der Review-Kommentare schlug fehl"
acked=$(printf '%s\n%s\n' "$acked_raw" "$acked_raw2" \
  | grep -oE 'acked-review:[[:space:]]*[0-9]+' | grep -oE '[0-9]+' | sort -u)

# try/catch um capture(): ohne ###-Ueberschrift wirft es, und der Fehler riss
# die gesamte Abfrage mit — jeder Zusammenfassungs-Befund dieses Laufs waere
# stillschweigend verschwunden. Auch .body kann null sein.
pending=$(gh api "repos/{owner}/{repo}/pulls/$pr/reviews" --paginate \
  --jq '.[]
        | select(.state != "APPROVED" and .state != "DISMISSED")
        | select(((.body // "")) | test("Changes recommended|Needs a closer look"))
        | "\(.id)\t\(.user.login)\t\((try ((.body // "") | capture("### [^\n]*\n+(?<head>[^\n]+)").head) catch null) // "siehe Zusammenfassung")"' \
  2>/dev/null) || unverifiable "die Abfrage der Reviews schlug fehl"

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

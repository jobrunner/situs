# Index im laufenden Betrieb tauschen

`serve` öffnet den Index **read-only**. Daraus folgt das ganze Verfahren auf
dieser Seite — und drei Eigenschaften, die es überhaupt erst möglich machen:

- **Nichts kann schreiben.** Kein verirrtes Statement, kein Schema-Einspielen,
  kein Journal-Moduswechsel; SQLite weist sie am Dateihandle ab.
- **Ein fehlender Index ist ein Startfehler**, keine neue leere Datei. Ein
  Tippfehler in `SITUS_INDEX_PATH` bricht den Start ab, statt einen Dienst zu
  liefern, der bei grünem `/health/ready` auf jede Abfrage `NOT_FOUND` sagt.
- **Keine Beiwagendateien.** Der Ingest hinterlässt den Index ohne WAL, `serve`
  legt kein `-wal`/`-shm` an. Der Index ist **eine Datei**, das Verzeichnis darf
  schreibgeschützt sein, und beim Stoppen des Containers ist nichts freizugeben.

## Das Verfahren

```bash
# 1. Index daneben legen, nicht darüber
scp situs-neu.sqlite dockerhost:/srv/situs/data/situs.sqlite.neu

# 2. In einem Schritt an seinen Platz ziehen
ssh dockerhost 'mv /srv/situs/data/situs.sqlite.neu /srv/situs/data/situs.sqlite'

# 3. Image ziehen und Container ERSETZEN, wie bei jedem anderen Deploy
ssh dockerhost 'docker compose pull situs && docker compose up -d --force-recreate situs'
```

`--force-recreate` ist nicht optional: hat sich nur der Index geändert und nicht
das Image, lässt `docker compose up -d` den laufenden Container stehen — und der
bedient, wie unten beschrieben, weiter seine alte Inode. Der Tausch käme nie an.

Schritt 1 und 2 sind **zwei** Schritte, und das ist der Punkt: `mv` innerhalb
desselben Verzeichnisses ist ein atomarer Rename. Niemand sieht je eine halbe
Datei — jede Antwort kommt aus einem vollständigen Index.

Aus **welchem** vollständigen Index, ist im Fenster zwischen Schritt 2 und 3
allerdings nicht festgelegt: die Verbindungen, die der laufende Prozess schon
offen hat, bedienen weiter die alte Inode; öffnet der Pool unter Last eine
weitere Verbindung, landet die bei der neuen Datei. Ein Prozess kann in diesen
Sekunden also beide Stände bedienen. Genau deshalb steht Schritt 3 unmittelbar
dahinter — und deshalb ist die Reihenfolge „erst tauschen, dann Container
ersetzen" nur so lange richtig, wie zwischen beiden nichts anderes passiert.

!!! warning "Nicht über die servierte Datei kopieren"

    Ein `scp` direkt auf `situs.sqlite` kürzt die Datei zuerst auf null und
    schreibt sie über Sekunden voll. Eine Anfrage, die in dieses Fenster fällt,
    sieht eine halbe Datenbank — das gibt `database disk image is malformed`
    oder schlicht falsche Zählungen. Kaputt geht dabei nichts, weil niemand
    schreibt, und nach dem Containertausch ist wieder alles sauber; aber der
    alte Container antwortet währenddessen möglicherweise mit Fehlern. Die
    zwei Schritte oben kosten ein Kommando mehr und haben dieses Fenster nicht.

!!! warning "Das Verzeichnis mounten, nicht die Datei"

    ```yaml
    volumes:
      - /srv/situs/data:/data        # richtig
      - /srv/situs/data/situs.sqlite:/data/situs.sqlite   # falsch
    ```

    Ein Bind-Mount auf eine **Datei** bindet deren Inode. Der Rename aus
    Schritt 2 erzeugt eine neue Inode — im Container käme er nie an, auch nicht
    nach einem Neustart des Prozesses.

## Warum der Prozess den Tausch nicht selbst bemerkt

Er bemerkt ihn nicht, und das ist kein Versäumnis: SQLite hält geöffnete Seiten
im Cache, und eine Datei, die unter ihm ersetzt wird, ist für ein offenes
Handle nicht verlässlich sichtbar. Ein `mv` ändert die Inode — der Prozess
bedient weiter die alte Datei, die nur noch er sieht. Der Containertausch ist
das, was den neuen Index in den Dienst bringt, und bei einem Dienst, der in
Millisekunden startet, ist das der einfachere und ehrlichere Weg als eine
Überwachungslogik im Binary.

Deshalb ist auch `immutable=1` bewusst **nicht** gesetzt: es würde SQLites
eigene Wechselerkennung abschalten, die hier die zweite Absicherung ist.

## Nicht in den servierten Index ingestieren

`situs ingest --db` schreibt read-write **in genau den Pfad, den es bekommt** —
und ohne `--db` ist das `index.path`, also die servierte Datei. Läuft dabei ein
`serve` darauf, liest dieses aus einem Index, der sich gerade unter ihm ändert:
dieselbe halbe Datei wie beim `scp` oben, nur über Minuten.

Das verhindert kein Schalter, sondern nur die Aufrufkonvention: **immer einen
neuen Nachbarpfad angeben** und ihn erst nach dem Lauf an seinen Platz ziehen.

```bash
situs ingest --csv-dir out/ingest-input --db /srv/situs/data/situs.sqlite.neu
mv /srv/situs/data/situs.sqlite.neu /srv/situs/data/situs.sqlite
```

## Pfade mit Sonderzeichen

Ein Indexpfad darf `?`, `#` und `%` enthalten — situs escapt sie, bevor der Pfad
in die SQLite-URI geht. Das ist kein Schönheitsdetail: unescapt schneidet der
Treiber die DSN am ersten `?` ab (gemessen gegen `modernc.org/sqlite` v1.56.0),
und SQLite prozent-dekodiert den Dateinamen. Ein Verzeichnis `feature%2Fbranch`,
wie es CI-Checkouts anlegen, zeigte damit auf einen Pfad, den es nicht gibt.

## Was `serve` beim Start ablehnt

- **Kein Index an dem Pfad** — statt einen leeren anzulegen.
- **Eine Datei, die kein situs-Index ist**: eine leere Datei, eine abgebrochene
  Kopie oder eine fremde Datenbank. `serve` prüft beim Öffnen die Tabelle, die
  jeder Lesepfad braucht, und bricht ab, statt bei grünem `/health/ready` auf
  jede Abfrage `INTERNAL_ERROR` zu antworten.
- **Ein Index mit WAL-Modus auf einem schreibgeschützten Verzeichnis** — der
  braucht die `-shm`-Beiwagendatei, und ein `:ro`-Mount kann keine haben. Das
  trifft **jeden Index, der vor 0.11.0 gebaut wurde**; siehe den Abschnitt
  gleich darunter.

- **Ein Index mit einer liegengebliebenen `-journal`/`-wal`-Datei** eines
  abgestürzten Schreibers. Die muss zurückgerollt werden, bevor irgendetwas
  gelesen werden darf, und das ist ein Schreibvorgang. Die Beiwagendatei zu
  löschen ist **nicht** der Ausweg — sie ist die Aufzeichnung dessen, was noch
  rückgängig zu machen ist. Einmal mit beschreibbarem Verzeichnis starten, dann
  erledigt SQLite es selbst.

Welchen SQLite-Code der WAL-Fall auslöst, hängt an der Umgebung und nicht an
der Ursache: ein `chmod`-geschütztes Verzeichnis meldet
`SQLITE_READONLY_DIRECTORY (1544)`, ein `:ro`-Bind-Mount meldet
`SQLITE_CANTOPEN (14)` — derselbe Fehler, den auch eine fehlende Datei
auslöst. `serve` liest deshalb den Dateikopf und sagt, was wirklich los ist,
statt aus dem Code zu raten.

## Upgrade von vor 0.11.0: den Index einmal finalisieren

Bis 0.10.x hinterließ der Ingest den Index im **WAL-Modus**. `serve` öffnete ihn
damals read-write und legte die Beiwagen selbst an; seit 0.11.0 öffnet es
read-only und kann das nicht mehr. Ein solcher Index bricht den Start ab:

```
Error: opening the index "/db/situs.sqlite": opening sqlite index
"/db/situs.sqlite" read-only (this index is still in WAL mode, and a read-only
handle on it needs a -shm sidecar it cannot create here — …): unable to open
database file (14)
```

Zwei Wege heraus. Der gründliche ist ein frischer `situs ingest` mit 0.11.0 — er
finalisiert am Ende selbst. Der schnelle ist eine Zeile auf dem Host, wo die
Datei beschreibbar ist, bei gestopptem Container:

```bash
sqlite3 /pfad/situs.sqlite 'PRAGMA wal_checkpoint(TRUNCATE); PRAGMA journal_mode=DELETE;'
# Ausgabe muss auf "delete" enden. Steht dort "wal", war die Datenbank belegt.
rm -f /pfad/situs.sqlite-shm
```

**Die Ausgabe ist die Prüfung, nicht der Exit-Code.** `journal_mode=DELETE`
antwortet mit dem Modus, der danach gilt — steht dort weiter `wal`, hielt noch
jemand die Datenbank offen, und dann darf **nichts** gelöscht werden: die
`-wal` trägt in dem Fall Daten, die noch nicht in der Datenbank stehen.

Nach einem erfolgreichen Wechsel ist die `-wal` schon weg (gemessen), die
`-shm` bleibt liegen und ist bedeutungslos — sie trägt nur den WAL-Index im
Shared Memory, keine Daten. Ein `serve` stört sie nicht; das `rm` ist
Kosmetik.

Ob ein Index betroffen ist, sagt Byte 18 seines Headers — `2` heißt WAL, `1`
heißt finalisiert:

```bash
xxd -s 18 -l 1 -p /pfad/situs.sqlite
```

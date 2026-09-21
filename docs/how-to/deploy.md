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

# 3. Image ziehen und Container ersetzen, wie bei jedem anderen Deploy
ssh dockerhost 'docker compose pull situs && docker compose up -d situs'
```

Schritt 1 und 2 sind **zwei** Schritte, und das ist der Punkt: `mv` innerhalb
desselben Verzeichnisses ist ein atomarer Rename. Der laufende Container behält
seine alte Inode und bedient bis zur letzten Sekunde ungestört den alten Stand;
der neue Container öffnet die neue Datei. Es gibt keinen Moment, in dem jemand
eine halbe Datei sieht.

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

`situs ingest --db` schreibt read-write. Läuft dabei ein `serve` auf derselben
Datei, liest dieses aus einem Index, der sich gerade unter ihm ändert — dieselbe
halbe Datei wie beim `scp` oben, nur länger. Der Ingest baut deshalb immer eine
**neue** Datei, die erst fertig an ihren Platz gezogen wird.

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
  braucht die `-shm`-Beiwagendatei. Das trifft Indizes, die vor diesem Release
  gebaut wurden; die Fehlermeldung nennt den `situs ingest`, der sie finalisiert.

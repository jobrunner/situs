# Changelog

## [0.12.0](https://github.com/jobrunner/situs/compare/v0.11.1...v0.12.0) (2026-09-22)


### ⚠ BREAKING CHANGES

* **syntaxa:** Syntaxon-IDs folgen dem EVC-Primaercode (AA01A) statt dem EEA-Code (PAP-01A). Anders als zunaechst dokumentiert bleibt die alte ID nutzbar: sie steht im Feld eea_code, und GET /v1/syntaxon/{id} nimmt sie weiterhin an. Das Feld hiess in einer frueheren Fassung dieses Branches alt_code.
* **api:** Syntaxon-IDs folgen jetzt dem EVC-Primaercode (AA01A) statt dem EEA-Code (PAP-01A). Die alte ID steht im neuen Feld alt_code. GET /v1/syntaxon/{id}/habitat-types beantwortet eine alte ID mit NOT_FOUND.

### Features

* **api:** /v1/areas nimmt scheme, Vorgabe wgsrpd_l3 ([fab00b4](https://github.com/jobrunner/situs/commit/fab00b4bc506875345edd5e160d6a8d1d9f53006))
* **api:** /v1/info misst Syntaxa-Gebietsschema und Abdeckung ([bab7fd7](https://github.com/jobrunner/situs/commit/bab7fd7d911969ec8a409e104c08daa982adff45))
* **api:** /v1/syntaxa nimmt area und include, Unbeurteilbare bleiben unmarkiert stehen ([a5600af](https://github.com/jobrunner/situs/commit/a5600af159a6693ebb3e93549ea1db23e2218f24))
* **api:** SyntaxonDetail traegt die Verbreitungsaussage, fehlt ohne Abdeckung ([e4989ac](https://github.com/jobrunner/situs/commit/e4989acb93ac989a61932d3a779b9803eb1626d4))
* **application:** Syntaxon-Detail mit Ahnenpfad und Kindern, Liste je Rang ([84d9729](https://github.com/jobrunner/situs/commit/84d97297649e806daf22e06f3c2fe492e5b5c35e))
* **data:** Formationsebene A-Y als Datendatei, Hierarchie-CSV wird Pflichtquelle ([8fb4656](https://github.com/jobrunner/situs/commit/8fb465686e33a52ef830687389e98d6df8749318))
* **domain,sqlite:** Syntaxon um Altcode, Quelle, Elternteil-Provenienz und Lebensform ([f99aabd](https://github.com/jobrunner/situs/commit/f99aabd012aea783faadadf2c48a7c65257412c9))
* **domain,sqlite:** zweites Gebietsschema und die zwei Verbreitungstabellen ([0dcbf59](https://github.com/jobrunner/situs/commit/0dcbf59f3dbec60b91e3dc50b57278597775fa6e))
* **eurovegchecklist:** Altcode ausgeben und Zellen ueber ihren Bezug lesen ([86f4c31](https://github.com/jobrunner/situs/commit/86f4c31cebaf65ae003497b62675dd263000fb45))
* **evc-distribution:** Quelle pinnen, Pipeline verdrahten, gemessene Zahlen dokumentieren ([d999f68](https://github.com/jobrunner/situs/commit/d999f684d0c9f5fb502cf81ffbe7bb59b996fe50))
* **evc-distribution:** Verbreitungstabelle ueber den Zellbezug in drei CSVs ([12400ad](https://github.com/jobrunner/situs/commit/12400ad8ac2620f84ca561c8d05144e0c12304ae))
* **explorer:** Panel Syntaxa-Navigation mit Brotkrumen und Kinderliste ([f5245a2](https://github.com/jobrunner/situs/commit/f5245a2a3b51973c3d928a113e5e8bdaccb287ab))
* **http:** Routen /v1/syntaxa und /v1/syntaxon/{id} samt OpenAPI-Vertrag ([d502bf9](https://github.com/jobrunner/situs/commit/d502bf99753b43a4cf2c9ff1cd2145797a65421c))
* **ingest,api:** IngestSyntaxa verdrahten, Herkunftsfelder ausliefern ([6aa1ff7](https://github.com/jobrunner/situs/commit/6aa1ff794952bb30d5cfa3e8839fc85634d16ad7))
* **ingest:** EEA-eigene Syntaxa erhalten, Kanten ueber den Altcode aufloesen ([59b52f8](https://github.com/jobrunner/situs/commit/59b52f8ae5b2141a5cfd35ca8ea2a24fb0d688ac))
* **ingest:** Formationen und EVC-Hierarchie als primaere Syntaxa-Quelle ([de40071](https://github.com/jobrunner/situs/commit/de40071d793e7dcb17c68445bb7196e183d107b7))
* **ingest:** Restelternteile per Namensabgleich und Geschwisterkonsens, Waisen brechen ab ([fe168b3](https://github.com/jobrunner/situs/commit/fe168b3e14788d30637b473a9bfffc0f3ec46ede))
* **ingest:** Syntaxa-Verbreitung und Abdeckung aus CSV einlesen ([395219e](https://github.com/jobrunner/situs/commit/395219e6c50f6f93ed18fee591caac3b824d0c41))
* **ingest:** Territorien und Syntaxa-Verbreitung als lokale Overlays einhaengen ([e315176](https://github.com/jobrunner/situs/commit/e315176d08f6e7324bc0321cd7cc798c48abc5e6))
* **sqlite:** Kinder, Ahnenpfad und Kantenzahl eines Syntaxons lesen ([598ac56](https://github.com/jobrunner/situs/commit/598ac563ffd8ec502074f5a5b46c9bc6878caa95))
* **sqlite:** Syntaxa nach Rang und Lebensform-Gruppe lesen, Raenge aus dem Index ([a47267a](https://github.com/jobrunner/situs/commit/a47267ab0384c88e448dd727ba586ca1de1c65c3))
* **sqlite:** Syntaxa-Verbreitung schreiben und lesen, Gebietsabfragen je Schema ([a13b6f5](https://github.com/jobrunner/situs/commit/a13b6f5771dfa98e5354180c1d904fec4ac9a104))
* **sqlite:** Syntaxon-Felder schreiben, Elternteil setzen, Kanten umschreiben ([7e50837](https://github.com/jobrunner/situs/commit/7e5083735aab63bf32fcec3b714f46995f55b7e5))
* **syntaxa:** EEA-Code-Fallback auch fuer die Habitattyp-Kanten eines Syntaxons ([257791a](https://github.com/jobrunner/situs/commit/257791a52509f7d3f718bd06e8a0d04ea9647488))
* **syntaxa:** Syntaxon ueber seinen frueheren EEA-Code auffindbar machen ([ba7d0c9](https://github.com/jobrunner/situs/commit/ba7d0c9be4e6f2385480dd83fbf40bac787317bb))


### Bug Fixes

* **api:** area-Filter fuer rank=class und rank=order ablehnen ([43f6b27](https://github.com/jobrunner/situs/commit/43f6b27f541223902dfa0e8a47702add05bb2392))
* **eurovegchecklist:** Altcode-Kollision abbrechen statt nur zaehlen ([c952bc1](https://github.com/jobrunner/situs/commit/c952bc1985119f3361a6a9b93837e2b155a9762b))
* **eurovegchecklist:** Code-Kommentare ins Englische uebersetzen ([0413577](https://github.com/jobrunner/situs/commit/0413577763019bd726600d4379f5b890e7cf7e94))
* **evc-distribution:** Summenspalten-Grenze strukturell statt namentlich erkennen ([a1c4f3a](https://github.com/jobrunner/situs/commit/a1c4f3a65d5ecbe2e675dc2d9c0b5554d724691d))
* **ingest:** den ganzen Syntaxon-ID-Namensraum vor dem Schreiben pruefen ([78740a8](https://github.com/jobrunner/situs/commit/78740a8cecb632ded3306071bc8e0dfd7e1f304f))
* **ingest:** doppelten Primaercode auf beiden Seiten zurueckweisen ([968c3c0](https://github.com/jobrunner/situs/commit/968c3c0281e06f8f45703783d0e02c151ce94b80))
* **ingest:** EEA-Zeile ohne id verwerfen statt als Syntaxon zu schreiben ([20a966d](https://github.com/jobrunner/situs/commit/20a966d5fc12252ea76c4fcb1600b1a8b84c77c9))
* **ingest:** Elternteil vom falschen Rang den Ingest scheitern lassen ([b55f011](https://github.com/jobrunner/situs/commit/b55f011174c9abadf3a2f46eb24e252ef545d468))
* **ingest:** Formation mit Elternteil auf beiden Seiten schliessen ([8297fcd](https://github.com/jobrunner/situs/commit/8297fcdb1c5259921805f917e8f3ca02ebb964dd))
* **ingest:** Formationsbuchstaben als Schluessel pruefen statt als Spalte lesen ([c69b357](https://github.com/jobrunner/situs/commit/c69b3575a8505b4625e058c97cd9711ba08bc488))
* **ingest:** Gebietsnamen gegen die Menge der bekannten Schemata pruefen ([1a4a47d](https://github.com/jobrunner/situs/commit/1a4a47d4414bbafb6d8d4ac4bc6545777af6c8a1))
* **ingest:** Hierarchiezeile ohne code verwerfen statt namenlos schreiben ([71e0592](https://github.com/jobrunner/situs/commit/71e0592f2a2f8f6adba5048d2c2176be5ee11f34))
* **ingest:** jede Beanspruchung einer Syntaxon-ID zaehlen, nicht jede Quelle ([6305469](https://github.com/jobrunner/situs/commit/63054692e6830317ba1ea99273088c2177bbfffe))
* **ingest:** Kollidierende EEA-Codes vom Remapping ausschliessen statt Gewinner zu kueren ([4c9ccba](https://github.com/jobrunner/situs/commit/4c9ccba57e4e1a8a9f35e7aa0c90dcf26679ac27))
* **ingest:** Kollidierenden EEA-Code auch beim Elternabgleich ausschliessen ([4b391cb](https://github.com/jobrunner/situs/commit/4b391cb4444d8a1a2ca636891a29ccb46bf84f9d)), closes [#54](https://github.com/jobrunner/situs/issues/54)
* **ingest:** Kollidierenden EEA-Code den Ingest scheitern lassen ([ef50b28](https://github.com/jobrunner/situs/commit/ef50b2833ba158114114736bd3f291673a21b14c))
* **ingest:** leeren Formationssatz vor der Transaktion zurueckweisen ([0bda433](https://github.com/jobrunner/situs/commit/0bda433e2c24aba5c9a614d36cea4401664999e3))
* **ingest:** Mehrdeutigen Namensabgleich nicht mehr vom Geschwisterkonsens abhalten ([6532428](https://github.com/jobrunner/situs/commit/65324283fa82dcab7c305ca4db7775f3d0ecdc72))
* **ingest:** Ordnung/Verband an uebersprungener Klasse als Waise erkennen ([9cec1f6](https://github.com/jobrunner/situs/commit/9cec1f65ebe2f284a44c59dd56fecf321b843e91))
* **ingest:** Syntaxa vor dem Schreiben leeren statt zu upserten ([1c39029](https://github.com/jobrunner/situs/commit/1c39029990596dcf990bba85723c680f4384ec24))
* **ingest:** Syntaxa-Verbreitungstabellen beim Ingest ersetzen statt ergaenzen ([7cce1da](https://github.com/jobrunner/situs/commit/7cce1da4f2c35df8578a1a2d4f0e7f9abe75e58d)), closes [#53](https://github.com/jobrunner/situs/issues/53)
* **ingest:** Timeout einer Konzeptanfrage nicht als Abbruch des Laufs deuten ([643f381](https://github.com/jobrunner/situs/commit/643f38160e9aae5dbd6a8990ad2155d5b13d56da))
* **ingest:** Verbreitungszeile ohne Coverage-Zeile den Ingest scheitern lassen ([ddc3642](https://github.com/jobrunner/situs/commit/ddc36421ea03f2e578546ac29f29164dd887b1b3))
* **ingest:** Verbreitungszeilen nur fuer Verbaende schreiben ([6ae735f](https://github.com/jobrunner/situs/commit/6ae735f27d8b18afad5e9691c5500719cdb24c00))
* **pipelines:** doppelten Verbandscode in evc-distribution zurueckweisen ([22e6b52](https://github.com/jobrunner/situs/commit/22e6b5271b4d7e5422865e03a51ea63cffdb6135))
* **pipelines:** eurovegchecklist raeumt out/ vor dem Lauf ([31bd577](https://github.com/jobrunner/situs/commit/31bd5774dba1b2fae11e300f59fa362538daf717))
* **pipelines:** evc-distribution bei null Verbaenden scheitern lassen ([1612e1a](https://github.com/jobrunner/situs/commit/1612e1a518d26af2c96ef3e86ba0ddca69cdf96c))
* **pipelines:** evc-distribution raeumt out/ vor dem Lauf, statt alte CSVs stehen zu lassen ([463713a](https://github.com/jobrunner/situs/commit/463713a0751d49e19efaaa9a11c85926d528ac12))
* **pipelines:** Slug-Kollision auch bei identischen Spaltenueberschriften abbrechen ([acf8cba](https://github.com/jobrunner/situs/commit/acf8cbab5b7fa025ddc667f00cac7bf1b7310fdf))
* **serve:** SyntaxonByEEACode raet bei mehrdeutigem Index nicht mehr ([dd5687a](https://github.com/jobrunner/situs/commit/dd5687accb533e79d1c7b804a26f488ccb39952d))
* **sqlite:** CTE-Schrittgrenze gegen maxSyntaxonAncestors testen ([fdaea8f](https://github.com/jobrunner/situs/commit/fdaea8f4beec653a81dd018863a04609dca0b428))
* **sqlite:** SetSyntaxonParent auf eine unbekannte ID scheitern lassen ([383053c](https://github.com/jobrunner/situs/commit/383053c5eb59cb66f58a07f25cce0f0ccf5a600e))
* **syntaxa:** EEA-Code-Fallback verschluckt keine echten Fehler mehr ([032432c](https://github.com/jobrunner/situs/commit/032432ccd41c8b06c67739552f7e8bc74bc7f117))
* **syntaxa:** Formationen tragen ParentProvenance official ([2a72052](https://github.com/jobrunner/situs/commit/2a72052959d6bf996e680618ed939558ac994e71))
* **syntaxa:** Verbreitungs-Ingest verlangt evc_territory statt jedes bekannten Schemas ([149dba6](https://github.com/jobrunner/situs/commit/149dba6440f6163e6d6791099b006a8d7326ead1))
* **syntaxa:** Zykel im parent_code werden beim Ingest erkannt ([4280c6b](https://github.com/jobrunner/situs/commit/4280c6b4d11d9d95c03da480bb098a8cde59e448))


### Documentation

* **api:** Wechsel des Syntaxon-Identifikators dokumentieren ([b98587e](https://github.com/jobrunner/situs/commit/b98587e2d726651f93a5b78487eaf1e86c469154))

## [0.11.1](https://github.com/jobrunner/situs/compare/v0.11.0...v0.11.1) (2026-09-21)


### Bug Fixes

* **serve:** Diagnose dem Symlink folgen lassen, Beiwagen-Fall benennen ([0150fe0](https://github.com/jobrunner/situs/commit/0150fe0a7aacf7ba655c4930059f23d4fa10e3a4))
* **serve:** WAL-Index beim Start benennen statt Dateirechte zu vermuten ([8fe3f69](https://github.com/jobrunner/situs/commit/8fe3f698ccdfd0631c6c8b5d7ea755c2370c4b39))
* **serve:** WAL-Index beim Start benennen statt Dateirechte zu vermuten ([6bc9f23](https://github.com/jobrunner/situs/commit/6bc9f23801f411342eda0a3e88529ca8564006a1))

## [0.11.0](https://github.com/jobrunner/situs/compare/v0.10.0...v0.11.0) (2026-09-21)


### Features

* **data:** deutsche Namen für die EUNIS-Level 1 und 2 ([5dee1c3](https://github.com/jobrunner/situs/commit/5dee1c3c05a6fd2c0c6bd86856b15849ecab6ef6))
* **data:** deutsche Namen für die EUNIS-Level 1 und 2 ([eb73606](https://github.com/jobrunner/situs/commit/eb73606e76ce701ba8d4657debd381f984d68e9f))
* **serve:** Index read-only öffnen, Ingest hinterlässt eine Datei ([6c7bf38](https://github.com/jobrunner/situs/commit/6c7bf38da568202b67c1d9fdfdae719bb7d8739e))
* **serve:** Index read-only öffnen, Ingest hinterlässt eine Datei ([389ee7d](https://github.com/jobrunner/situs/commit/389ee7ddcdd03680746a9adaa316a17671247827))


### Bug Fixes

* **data:** zu enge und zu weite Volksnamen auf Level 1/2 entfernen ([298bac1](https://github.com/jobrunner/situs/commit/298bac1cc85a937c791c621b054fc58d448af91b))
* **deploy:** Container erzwungen ersetzen, Traversierrechte nennen ([c1a193d](https://github.com/jobrunner/situs/commit/c1a193da24c4af44a62d77140e0c7608f0f3fefa))
* **serve:** vollständiges Schema prüfen statt einer Tabelle ([ce9fcdf](https://github.com/jobrunner/situs/commit/ce9fcdf942974742f2f70b92e13b173e2d3c1ff1))

## [0.10.0](https://github.com/jobrunner/situs/compare/v0.9.0...v0.10.0) (2026-09-20)


### Features

* **ingest:** Eingabeverzeichnis per Befehl sammeln statt per Kopierliste ([6d46c35](https://github.com/jobrunner/situs/commit/6d46c35107730af7fa2efe4a12543391139c2921))
* **ingest:** Eingabeverzeichnis per Befehl sammeln statt per Kopierliste ([af4b77a](https://github.com/jobrunner/situs/commit/af4b77aae15599b4da23e1890c8625c54380ddb8))


### Bug Fixes

* **ingest:** Review-Funde aus PR [#43](https://github.com/jobrunner/situs/issues/43) ([c7edcf6](https://github.com/jobrunner/situs/commit/c7edcf6b875df0b1e96058fd3cd00cc6d4f5e6c7))

## [0.9.0](https://github.com/jobrunner/situs/compare/v0.8.0...v0.9.0) (2026-09-20)


### Features

* **annex1:** Beschreibungen der Lebensraumtypen, Fundament und vier Muster ([a219250](https://github.com/jobrunner/situs/commit/a219250f54a4a62b1c514807be7a91c52a76c6b5))
* **annex1:** Beschreibungen der Lebensraumtypen, Fundament und vier Muster ([ead8277](https://github.com/jobrunner/situs/commit/ead8277688b6f352d1d2cee00a656cc9fb7af78c))
* **annex1:** die übrigen 201 Beschreibungen, englisch und deutsch ([ee725b9](https://github.com/jobrunner/situs/commit/ee725b9164db69bfe69888acc522391cae26fbba))
* **annex1:** die übrigen 201 Beschreibungen, englisch und deutsch ([091de1f](https://github.com/jobrunner/situs/commit/091de1f69b09827dd4dc0ea035a5f89ffc3003b5))


### Bug Fixes

* **annex1:** CHECK auch in der Migration, Dubletten aus dem Rohlauf melden ([9670157](https://github.com/jobrunner/situs/commit/9670157634e8432030e61b9cbe7d4033a148ae81))
* **annex1:** Review-Funde aus PR [#40](https://github.com/jobrunner/situs/issues/40), vor allem am Parser ([79d5aec](https://github.com/jobrunner/situs/commit/79d5aec7a9adeb8cb05aa2bcab59d0ed6b9386cf))
* **annex1:** Review-Funde aus PR [#42](https://github.com/jobrunner/situs/issues/42) ([6474d16](https://github.com/jobrunner/situs/commit/6474d169713baeabf4e9710b4e065ecb47f91d9d))

## [0.8.0](https://github.com/jobrunner/situs/compare/v0.7.0...v0.8.0) (2026-09-20)


### Features

* **descriptions:** Habitatbeschreibungen aus dem EUNIS-ESy-Factsheet-PDF ([cca200d](https://github.com/jobrunner/situs/commit/cca200d7e48354709d68911a67ec25374dd94b48))
* **descriptions:** Habitatbeschreibungen aus dem EUNIS-ESy-Factsheet-PDF ([dfb8e80](https://github.com/jobrunner/situs/commit/dfb8e8022c82bd63c8b1f267ee3340ed33ecd17a))


### Bug Fixes

* **descriptions:** Prüfsumme portabel berechnen ([fc1cdb2](https://github.com/jobrunner/situs/commit/fc1cdb2476b448b4e20ae1a1c7de8999e58c7243))
* **descriptions:** Review-Funde aus PR [#38](https://github.com/jobrunner/situs/issues/38) ([25ce4fc](https://github.com/jobrunner/situs/commit/25ce4fc9426b94ddf5f94e153f79be966c7a2219))

## [0.7.0](https://github.com/jobrunner/situs/compare/v0.6.0...v0.7.0) (2026-09-15)


### Features

* **areas:** GET /v1/areas mit ingesteten WGSRPD-Gebietsnamen ([38d6b7d](https://github.com/jobrunner/situs/commit/38d6b7db5d380b064793cc9d7525bd3ea514cf87))
* **areas:** GET /v1/areas mit ingesteten WGSRPD-Gebietsnamen ([c9d6b72](https://github.com/jobrunner/situs/commit/c9d6b721e2bfed7bced4e3f27ab42ca100b56408))


### Bug Fixes

* **areas:** bare relativen CSV-Pfad lesen, Docstring-Beispiele korrigieren ([dc07e9f](https://github.com/jobrunner/situs/commit/dc07e9f38a0a9362370a27df950e9116af407727))
* **areas:** Review-Funde aus PR [#34](https://github.com/jobrunner/situs/issues/34) ([45ee0cc](https://github.com/jobrunner/situs/commit/45ee0ccd91a1029217657c1e6798726783f55d58))

## [0.6.0](https://github.com/jobrunner/situs/compare/v0.5.0...v0.6.0) (2026-09-15)


### Features

* **explorer:** fill typology selects from GET /v1/typologies ([c826b38](https://github.com/jobrunner/situs/commit/c826b38881a8394190ac88240e6e0cc3d1570cf2))
* **http:** GET /v1/typologies — die Habitat-Typologien des Index auflisten ([6ca07af](https://github.com/jobrunner/situs/commit/6ca07af19a164dfc75668ba3df52d50f4cb3dda6))
* **http:** GET /v1/typologies lists every registered typology ([38a81cc](https://github.com/jobrunner/situs/commit/38a81ccc095935f2de484e16db45c174f88bbbcf))

## [0.5.0](https://github.com/jobrunner/situs/compare/v0.4.0...v0.5.0) (2026-09-15)


### Features

* **config:** CORSConfig with a registered viper default ([c6c6f44](https://github.com/jobrunner/situs/commit/c6c6f4444d54b59608861e1df58f23b0f5e37920))
* **domain:** Origin and OriginPattern value objects for CORS allow-listing ([2c4ee46](https://github.com/jobrunner/situs/commit/2c4ee46dfaf0d07ed6277114d13c9fec40bc1264))
* **http:** optional CORS, off unless an origin is configured ([e07a53e](https://github.com/jobrunner/situs/commit/e07a53ee93e4edef6e053a77b149f1fa801227d1))
* **http:** optionales CORS, standardmäßig aus ([d51688b](https://github.com/jobrunner/situs/commit/d51688bbaaff99409532402664652206ab89289c))


### Bug Fixes

* close four CORS review findings ([d8f8213](https://github.com/jobrunner/situs/commit/d8f821350ae785f01e799ac6ec809d47cc14e777))
* close three CORS review findings (wiring test, wildcard advice, case) ([3902322](https://github.com/jobrunner/situs/commit/3902322e4af729301e653c1bbb80839db1917883))
* **domain:** close three more CORS allow-list entries that can never match ([5e7fd42](https://github.com/jobrunner/situs/commit/5e7fd429dfb63ca910ebe494e943a4ff343767f0))
* fuzzer for CORS origin patterns + four review findings ([6a80f1c](https://github.com/jobrunner/situs/commit/6a80f1cacb3f330422f7a2750cbaaee06fa4b733))
* **http,domain:** close three CORS caching/parsing gaps from review ([ba829af](https://github.com/jobrunner/situs/commit/ba829af5411af50276847ba6a1eb752531a5d51f))
* **http:** log the aggregate consequence when every CORS origin is unusable ([b5d2447](https://github.com/jobrunner/situs/commit/b5d2447f1763e43fd9e2653379b3bfe79a47ac1c))

## [0.4.0](https://github.com/jobrunner/situs/compare/v0.3.0...v0.4.0) (2026-09-14)


### Features

* API-Explorer unter / plus Namenssuche und Zeigerwertanalyse ([00c4416](https://github.com/jobrunner/situs/commit/00c441625da17305980024da7e6d30341ef7b494))
* **application:** pure indicator-value statistics over a species list ([16fd974](https://github.com/jobrunner/situs/commit/16fd97410e1b1ff281172673eb3c3da37ad89bda))
* **http:** GET /v1/species/search over the index's own names ([dc245d1](https://github.com/jobrunner/situs/commit/dc245d1e54d2317820228c44dbcc59c20a009407))
* **http:** POST /v1/species/traits/summary — indicator-value analysis ([5264375](https://github.com/jobrunner/situs/commit/52643753670b34504003ed8be4a5662d9beb3a9e))
* **http:** self-contained API explorer at / ([cf78b71](https://github.com/jobrunner/situs/commit/cf78b71c755f6bf6b7ad6d56640fbc0ebc2a5668))
* **sqlite:** search the index's own verbatim species names ([8d120aa](https://github.com/jobrunner/situs/commit/8d120aac8ff44c7d64d5831cbe83e4cad62d6b9c))
* **sqlite:** TraitsForConcepts reads many concepts in one query ([93f776e](https://github.com/jobrunner/situs/commit/93f776e5e68a99b2058d870f3221cb8d54dc7bda))


### Bug Fixes

* address PR [#28](https://github.com/jobrunner/situs/issues/28) review findings round 2 ([03a7859](https://github.com/jobrunner/situs/commit/03a78594c55839445f6a70a0307c1391bac165c9))
* **application:** guard trait summary against mixed vocab versions ([ce4f696](https://github.com/jobrunner/situs/commit/ce4f69616a8aeb9cc16668ecbc91c710eca6f8da))
* **application:** match fakeRepo.SearchSpeciesNames to the real adapter ([50c56a9](https://github.com/jobrunner/situs/commit/50c56a97a791d679ecacb4c94975b8bfab403c5a))
* **explorer:** live-search race, keyboard access, and empty ?limit= ([e0f6160](https://github.com/jobrunner/situs/commit/e0f6160aaeaf326198a7fe006c3e4037c07b3105))
* **http:** move q/limit validation to the use case, keep only parsability in the handler ([8221a53](https://github.com/jobrunner/situs/commit/8221a53c908d81b25f4cd05fadfe5e49ca3698bf))
* **http:** reject explicit limit=0 as INVALID_QUERY, dedupe concept_ids body decode ([13b1797](https://github.com/jobrunner/situs/commit/13b1797180104cd0bc9492a41f0d89c23793c50e))
* **review:** tighten OpenAPI q/concept_ids schemas, catch explorer fetches, annotate stale plan signature ([75cf2bc](https://github.com/jobrunner/situs/commit/75cf2bcab66005752d5b7be40c9b175924be5b0a))

## [0.3.0](https://github.com/jobrunner/situs/compare/v0.2.0...v0.3.0) (2026-08-30)


### Features

* Aggregat-Mitgliedsarten via dateibasiertem Species-Ingest ([00590e9](https://github.com/jobrunner/situs/commit/00590e9560625eacfb0b9cb0677d3bd48c6508f2))
* Aggregat-Mitgliedsarten via dateibasiertem Species-Ingest ([815fe4b](https://github.com/jobrunner/situs/commit/815fe4b76aeac798cc8c42fc5f8f26746d40f5a5))
* **application:** IngestSyntaxaHierarchy matches EUNIS alliances to FloraVeg ([f63fc9d](https://github.com/jobrunner/situs/commit/f63fc9d7317f4bf9fcf8d5227f2e4092b83c63c8))
* **application:** IngestTraits — one resolver call across three vocabularies ([d5f1ef4](https://github.com/jobrunner/situs/commit/d5f1ef4155edb853ba05e8361a3af17fbe7dd2ed))
* **application:** resolve species roles from a local crosswalk file, derive aggregate members ([ca80ef5](https://github.com/jobrunner/situs/commit/ca80ef541f2ede9298e336b67b86db4a1e74ebce))
* **cmd:** wire IngestSyntaxaHierarchy into situs ingest ([a19203a](https://github.com/jobrunner/situs/commit/a19203a10526f17e32a49a1d3f5ee26f8313f5f7))
* **cmd:** wire IngestTraits into situs ingest ([9c0b421](https://github.com/jobrunner/situs/commit/9c0b421fa68210f5386db1dd54a685b5418acdc6))
* **cmd:** wire the crosswalk/aggregate-members flags into situs ingest ([fa9c019](https://github.com/jobrunner/situs/commit/fa9c01911ca9331d20b1599132348d630b85a347))
* **domain:** add SpeciesRole.Provenance/DerivedFrom and UpsertDerivedSpeciesRole ([29d6f9c](https://github.com/jobrunner/situs/commit/29d6f9ceb8af8856e16fb5da530db3758f022b82))
* **domain:** add Syntaxon.Author and AllSyntaxa/UpsertSyntaxonAuthor ([c3b18d7](https://github.com/jobrunner/situs/commit/c3b18d702a42acb61710fe1bbb6bc386d029f9d8))
* **domain:** TraitDim/TraitValue/TraitSet + IngestTx/Repository trait ports ([d9cfd55](https://github.com/jobrunner/situs/commit/d9cfd55681271403ed94db896a4041c2f83152e6))
* **http:** expose SpeciesEntry/HabitatTypeRole provenance/derived_from ([765f9b8](https://github.com/jobrunner/situs/commit/765f9b8500bd5d42c468c2387489ed9b5a2f34e7))
* **http:** expose SyntaxonRef.author/parent_id ([a0b74a2](https://github.com/jobrunner/situs/commit/a0b74a2ffb37ec55caa11380c3fc55ad8f8634ef))
* **http:** GET /v1/species/{conceptId}/traits ([6e9217e](https://github.com/jobrunner/situs/commit/6e9217e923ded7621b006caeea5b93756455de99))
* **pipelines:** eive/tichy/midolo trait pipelines from hostus transfer ([143546e](https://github.com/jobrunner/situs/commit/143546ee8f53679228ee061248436fccb92b93d9))
* **pipelines:** FloraVeg EuroVegChecklist -&gt; syntaxa_hierarchy.csv ([3e6e238](https://github.com/jobrunner/situs/commit/3e6e238e61b66e4925465564e3f104ec3b1b531c))
* **sqlite:** trait_value/trait_vocabulary tables + Traits/KnownVocabs ([144809f](https://github.com/jobrunner/situs/commit/144809f760dd9530aeb27f4b219351f70099f011))
* Syntaxa-Hierarchie (Klasse/Ordnung) via FloraVeg.EU + Autorschafts-Trennung ([6677a8f](https://github.com/jobrunner/situs/commit/6677a8fcc672f0ae350cca3b481c64453595101e))
* Syntaxa-Hierarchie (Klasse/Ordnung) via FloraVeg.EU + Autorschafts-Trennung ([0479c16](https://github.com/jobrunner/situs/commit/0479c16fca5bf2c3c4baf79dd7be9e9ca762f8a3))
* Trait-Modul (EIVE/Tichý/Midolo Zeigerwerte) ([107bc2e](https://github.com/jobrunner/situs/commit/107bc2ece1c84ab71e598af02c5fa731ca51ff53))


### Bug Fixes

* address Copilot review round 1 on PR [#21](https://github.com/jobrunner/situs/issues/21) ([a606fe8](https://github.com/jobrunner/situs/commit/a606fe819d6dd053c793049b21b98bdfafee0641))
* address Copilot review round 1 on PR [#25](https://github.com/jobrunner/situs/issues/25) ([cfc2347](https://github.com/jobrunner/situs/commit/cfc23477ffbc9f01494ef2362c7fdc85cc588420))
* address Copilot review round 1 on PR [#26](https://github.com/jobrunner/situs/issues/26) ([15350ea](https://github.com/jobrunner/situs/commit/15350eab1dcbad6784afa3117632e6f77b4c81e2))
* address Copilot review round 2 on PR [#21](https://github.com/jobrunner/situs/issues/21) ([34de730](https://github.com/jobrunner/situs/commit/34de730de421ed56fca0b674c809ce274f631b56))
* address Copilot review round 3 on PR [#21](https://github.com/jobrunner/situs/issues/21) ([7a1efcf](https://github.com/jobrunner/situs/commit/7a1efcf97988f7aacc316fc377367383ec4bec81))
* address Copilot review round 3 on PR [#26](https://github.com/jobrunner/situs/issues/26) ([24ae396](https://github.com/jobrunner/situs/commit/24ae3962455f82f0fb750b29e10ee302631aa0d7))
* address Copilot review round 4 on PR [#26](https://github.com/jobrunner/situs/issues/26) ([e1f3254](https://github.com/jobrunner/situs/commit/e1f3254e6f11ed38fa6e37594833427b7a8ca6f4))
* address GitHub Copilot review findings on PR [#20](https://github.com/jobrunner/situs/issues/20) ([7059f81](https://github.com/jobrunner/situs/commit/7059f81b00f1858be54bebec80642a5827392010))
* address whole-branch review findings on the trait module ([98265f2](https://github.com/jobrunner/situs/commit/98265f2299c1849148a1f5086c34eec82510e0ba))
* **application:** update syntaxon Name on a FloraVeg match, harden prefix match ([a6472c6](https://github.com/jobrunner/situs/commit/a6472c60a346fa5694a2ce8a61c2f1c4e12f0233))
* **ci:** App-token step tolerates a missing App secret ([8837c02](https://github.com/jobrunner/situs/commit/8837c02910e758064abc65fa59c3f1c9f4f36594))
* **ci:** surface a visible warning when the App token fails to mint ([f955256](https://github.com/jobrunner/situs/commit/f955256935066f77f9208c59b5beeb4e7e7a0b56))
* **domain:** translate leftover German comments to English ([8f16444](https://github.com/jobrunner/situs/commit/8f16444f73541ff429bf7f24f7b40bbbc0da9126))
* **lint:** resolve goconst/gocyclo findings from Task 1 and Task 4 ([145a077](https://github.com/jobrunner/situs/commit/145a077a9d11a44d70f4a0404441289cc0272daf))
* **pipelines:** fail fast on HTTP errors during source download ([287902c](https://github.com/jobrunner/situs/commit/287902cee641e89acacd16cca0aec21b47dd3999))

## [0.2.0](https://github.com/jobrunner/situs/compare/v0.1.1...v0.2.0) (2026-08-26)


### ⚠ BREAKING CHANGES

* **api:** name_de was a string with a sibling name_de_provenance; it is now an object with value, vernacular, provenance and source. Structural on purpose — a client cannot read the value without seeing what qualifies it, which is the whole point once some labels are official and others are situs' own translation. No consumer breaks: both old fields were omitempty and always absent at 0 localizations.
* **ports:** Localization returns every field of an entity
* **api:** batch habitat types by concept id, drop the runtime hostus dependency

### Features

* **api:** batch habitat types by concept id, drop the runtime hostus dependency ([011f3da](https://github.com/jobrunner/situs/commit/011f3da9484623c0270c8ad91f841479ed89a099))
* **api:** index self-description on /v1/info, and the docs sweep ([6bfbfa2](https://github.com/jobrunner/situs/commit/6bfbfa2ed690c222980803623a5d47f1f7092dde))
* **api:** mark and optionally filter species by area ([3befd7a](https://github.com/jobrunner/situs/commit/3befd7a07f52ecc5a7a9edff74308cec2f37fa4d))
* **api:** name_de becomes an object carrying its provenance ([f1f6482](https://github.com/jobrunner/situs/commit/f1f6482e8350bd2177563a20fb14abff62808ba9))
* **api:** serve an offline Swagger UI at GET /docs ([532be46](https://github.com/jobrunner/situs/commit/532be4621ddf06075eb37b7019e938bbc344dfb9))
* **api:** serve an offline Swagger UI at GET /docs ([210085f](https://github.com/jobrunner/situs/commit/210085fe85c7ffa8102544e8faada83f229d709e))
* **data:** 241 situs-authored German names for EUNIS level 3 ([3a5b54d](https://github.com/jobrunner/situs/commit/3a5b54df23f9bf5980f7cd5df04646fbb16c20bb))
* **domain:** area value object and the distribution ports ([5dd9101](https://github.com/jobrunner/situs/commit/5dd91018912f017e10aad4e5692b341ac65ea190))
* **hostus:** read species distribution per concept ([a09c730](https://github.com/jobrunner/situs/commit/a09c730a8b299b0b53da7d8284c83c739156c3d3))
* **ingest:** copy species distribution into the index ([3393d80](https://github.com/jobrunner/situs/commit/3393d80a0ee6b8e757ce36c2a595891356b8747a))
* **localization:** add the situs provenance value ([35b28b9](https://github.com/jobrunner/situs/commit/35b28b994064147d71651f719192bc0fb081a675))
* **pipeline:** official German Annex I names from EUR-Lex ([85f44b9](https://github.com/jobrunner/situs/commit/85f44b9859d6a69e004422beb3100211dbfc4f3f))
* **quality:** CodeCharta map as a third ratchet ([24d3623](https://github.com/jobrunner/situs/commit/24d362330cecd41dc346b9e875540ef88ac75be2))
* **quality:** CodeCharta-Map als dritter Ratchet ([17244bc](https://github.com/jobrunner/situs/commit/17244bc9f98c6ed73416ce03ce88e2ed5f3650fa))
* **sqlite:** species distribution table with idempotent writes and area reads ([32f8172](https://github.com/jobrunner/situs/commit/32f8172a77b25911d2d1fffca663a63b846c995b))


### Bug Fixes

* **api:** assert in_area on the batch route and reject blank concept ids ([eaa0b7c](https://github.com/jobrunner/situs/commit/eaa0b7cf3a60bf91a96fa73d96df6c6f219dd8ba))
* **api:** reject unparseable only_in_area, cover the filter wire-through, split area.go ([106ed08](https://github.com/jobrunner/situs/commit/106ed088e373cf9c49ef7c27c1a2f91239e55bde))
* **application:** fakeRepo.UpsertDistribution uses failIfNamed ([54d1f26](https://github.com/jobrunner/situs/commit/54d1f260c0f948e8312b9d80ce6abb7816b6ada7))
* **ci:** close the deferred harness minors from the branch review ([6d03d74](https://github.com/jobrunner/situs/commit/6d03d743c9e5af2637641609caa3f713cd6f2406))
* **ci:** close the deferred harness minors from the branch review ([75cfa39](https://github.com/jobrunner/situs/commit/75cfa392f81f97f2ae09aeaa322c14c79fadebda))
* **config:** default to port 8070 and make the spec server relative ([5063a39](https://github.com/jobrunner/situs/commit/5063a391f741bb2c9b5ccb2fd5f520f0a446d76f))
* **config:** default to port 8070 and make the spec server relative ([e9f802f](https://github.com/jobrunner/situs/commit/e9f802f3488c28e4229df78701b71bcdbd3d918c))
* **ingest:** move Failed to the command's report, warn on backbone drift ([334f55c](https://github.com/jobrunner/situs/commit/334f55c96780299d550b3ca0c52152ced9b2b1a7))
* **ingest:** review fixes for distribution ingest ([b1d1137](https://github.com/jobrunner/situs/commit/b1d1137a018aa1b4f8d8284eaaeefe17efd91c97))
* make the mutation gate actually generate mutants ([ee49359](https://github.com/jobrunner/situs/commit/ee4935936c1e8dac1c4debc73487b070ecae835e))
* Mutation-Gate erzeugt endlich Mutanten ([b0a838c](https://github.com/jobrunner/situs/commit/b0a838cfde63ea5f57ae61d9afa64d16b09d9e18))
* review findings on the German-label layer ([5be8a07](https://github.com/jobrunner/situs/commit/5be8a070873d874b38977a70be10a001f6a3fee3))
* review fixes for the mutation gate ([8428288](https://github.com/jobrunner/situs/commit/84282882fe23936536240b29ba22bbfcd5d00de5))


### Code Refactoring

* **ports:** Localization returns every field of an entity ([342ccc3](https://github.com/jobrunner/situs/commit/342ccc30860c908cab94699563d2711e88756a4e))

## [0.1.1](https://github.com/jobrunner/situs/compare/v0.1.0...v0.1.1) (2026-08-19)


### Bug Fixes

* **release:** push the ghcr image and publish the docs on every release ([637e4b3](https://github.com/jobrunner/situs/commit/637e4b3baf3e06f432a6d9e66bdc31ba4ad96d08))
* **release:** push the ghcr image and publish the docs on every release ([8b8b67a](https://github.com/jobrunner/situs/commit/8b8b67abd2dbb15e8f7559a4a87de6b3f43bdeb3))

## 0.1.0 (2026-08-19)


### Features

* **api:** habitat-type, species and syntaxon read endpoints ([bd7abea](https://github.com/jobrunner/situs/commit/bd7abeabde8185f7c9d5c7948a3f06e2f9692242))
* **domain:** admit the approximate qualifier instead of dropping its rows ([0c6155b](https://github.com/jobrunner/situs/commit/0c6155bb905aed7b19d5c49d0c6e2dd5faf2e421))
* **domain:** typology id, habitat type key and crosswalk qualifier ([0815848](https://github.com/jobrunner/situs/commit/08158486c23803d17e3082e62db999d647f825a8))
* **i18n:** german labels plus derived entry labels from '=' annex I crosswalks ([9d48b2d](https://github.com/jobrunner/situs/commit/9d48b2d7b2afc7c009e54026b413a7cfb3a3cd0d))
* **ingest:** load typologies, habitat types, crosswalks and syntaxa from csv ([2447219](https://github.com/jobrunner/situs/commit/244721982c8e7d9506cdaa2eddf0271510d2f4e7))
* **ingest:** species roles with hostus name crosswalk and measured resolution rate ([f4e51aa](https://github.com/jobrunner/situs/commit/f4e51aa322350671cb60c33fb34cc63a5ede37c9))
* **pipeline:** convert pinned EUNIS/ESy xlsx artifacts to normalized csv ([4273975](https://github.com/jobrunner/situs/commit/42739757df986636a96157d34e7e6150cf380bda))
* situs foundation — EUNIS-Habitattypen als lokaler read-only Dienst ([6f94296](https://github.com/jobrunner/situs/commit/6f942960c5fbbdba986c59cf2fa8c46a45b3f4c7))
* **sqlite:** habitat typology schema and idempotent ingest write side ([f10f2d4](https://github.com/jobrunner/situs/commit/f10f2d453f9cefa3226a5655e42f0cd9d87156ef))


### Bug Fixes

* **api:** make the batch contract true as written and guard the row ordering ([2111c4f](https://github.com/jobrunner/situs/commit/2111c4fbdde20735245485a8aa9b62fae2bf717c))
* bind in Docker, bound the batch endpoint, gate the pipeline tests ([ebaecf6](https://github.com/jobrunner/situs/commit/ebaecf66c862966f9ff9cb4d6f95f4ffac06aae7))
* **i18n:** prove the non-overwrite invariant and make the sqlite error paths deterministic ([856e48f](https://github.com/jobrunner/situs/commit/856e48f09dcbec7c558529660913bfceab507da5))
* **ingest:** batch hostus name resolution at 50, measured against real data ([8355490](https://github.com/jobrunner/situs/commit/835549080cfcd05836a014c1b2fef9cac4bd6e3e))
* **ingest:** make the hostus batch size configurable and degrade instead of failing ([75cfc08](https://github.com/jobrunner/situs/commit/75cfc08ac7dac5dde105dc2837164be9a6119c94))
* **ingest:** pin batching/id-mapping in hostus client, close review gaps ([8f02df6](https://github.com/jobrunner/situs/commit/8f02df66544ecb1becde48ae4dfdf28e6210f615))
* **ingest:** skip wrong-field-count rows, log through configured logger, check ctx per file ([41413f2](https://github.com/jobrunner/situs/commit/41413f29ba032b171b087497b6452ee495c346e7))
* **pipeline:** stop dropping the Man-made sheet and fail loudly on header drift ([3bef233](https://github.com/jobrunner/situs/commit/3bef233b1b9eff0146e59e679e089b3383ad77c3))
* route incidental adapter logs through the configured logger and reject trailing request data ([039ce89](https://github.com/jobrunner/situs/commit/039ce89f9cec928120bf0b711993686e65bf929e))
* wire the read timeout, gate coverage in verify, test the error envelope ([f199b7a](https://github.com/jobrunner/situs/commit/f199b7afe18f00d5ec07eebb8df6c7dac984233f))

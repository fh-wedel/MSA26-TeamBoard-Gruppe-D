thesis-template
===============

LaTeX-Vorlage für Projektbericht an der FH-Wedel
(Betreuer Ulrich Hoffmann)

PDF erzeugen:

    % cd projektbericht
    % pdflatex thesis_main.tex
    % biber thesis_main         # optional for bibliography
    % pdflatex thesis_main.tex  # several times in order to resolve open references

Alternative: Kompilierung per Docker
------------------------------------

Nur als Ausweichoption gedacht, falls lokal kein LaTeX (pdflatex/bibtex bzw.
biber) installiert ist. Benötigt eine laufende Docker-Installation. Der erste
Aufruf lädt ein mehrere GB großes TeX-Live-Image herunter.

Aus dem Ordner `docs/projektbericht`:

    docker build -t projektbericht .
    docker run --rm -v "${PWD}:/doc" projektbericht

Die fertige `thesis_main.pdf` liegt danach neben den Quelldateien. Der Container
führt `latexmk` aus, das pdflatex + biber inklusive der nötigen Wiederholungsläufe
erledigt.

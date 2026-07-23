# Build environment for the TeamBoard Projektbericht.
#
# Usage (run from docs/projektbericht):
#   docker build -t projektbericht .
#   docker run --rm -v "$PWD:/doc" projektbericht
#
# latexmk drives pdflatex + biber and reruns as needed to resolve references.
# The compiled thesis_main.pdf is written next to the sources on the host.
FROM texlive/texlive:latest

WORKDIR /doc

ENTRYPOINT ["latexmk", "-pdf", "-interaction=nonstopmode", "-halt-on-error", "thesis_main.tex"]

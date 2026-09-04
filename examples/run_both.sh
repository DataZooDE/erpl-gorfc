#!/usr/bin/env bash
#
# The same Go program, built against SAP's gorfc and against erpl-gorfc, with
# the outputs diffed. It compares *behaviour*, never source, which is what keeps
# this clean-room -- and it is the technique that found two real bugs on its
# first run for the Python binding, one of which the test suite could not see.
#
#   SAPNWRFC_HOME=/opt/nwrfcsdk/linux SAP_USER=DEVELOPER SAP_PASSWD=... \
#     bindings/gorfc/examples/run_both.sh
#
set -euo pipefail

# Hard-required: without the SDK the SAP side cannot be built, and the script
# would quietly compare erpl-gorfc against itself and always report no
# differences.
: "${SAPNWRFC_HOME:?set it to the unpacked SAP NW RFC SDK, e.g. /opt/nwrfcsdk/linux}"
: "${SAP_USER:?}"
: "${SAP_PASSWD:?}"

HERE="$(cd "$(dirname "$0")" && pwd)"
BINDING="$(cd "$HERE/.." && pwd)"
WORK="${WORK:-${TMPDIR:-/tmp}/gorfc-side-by-side}"
mkdir -p "$WORK"

build() {
  # Declared one per line: `local a=$1 b=$WORK/$a` expands $a before the
  # assignment lands, which under `set -u` is an unbound-variable error.
  local name="$1"
  local import="$2"
  local dir="$WORK/$name"
  mkdir -p "$dir"
  sed "s|GORFC_IMPORT_PATH|$import|" "$HERE/same_program.go.tmpl" > "$dir/main.go"
  ( cd "$dir" && go build -o program . )
}

# -- SAP's gorfc, against the SDK ------------------------------------------
#
# Built from a local checkout of the archived source rather than through the
# module proxy: `github.com/sap/gorfc` no longer resolves (the repository moved
# to SAP-archive), and a differential that cannot build the reference is worse
# than useless -- it would compare this library against itself.
#
# GORFC_SRC must point at a directory holding the reference gorfc.go, which is
#   curl -sSfLO https://raw.githubusercontent.com/SAP-archive/gorfc/master/gorfc/gorfc.go
: "${GORFC_SRC:?set it to a directory holding the reference gorfc.go}"
[ -f "$GORFC_SRC/gorfc.go" ] || { echo "no gorfc.go in $GORFC_SRC" >&2; exit 1; }

echo "== building against SAP gorfc (SAPNWRFC_HOME=$SAPNWRFC_HOME)"
mkdir -p "$WORK/sapmod/gorfc" "$WORK/sap"
cp "$GORFC_SRC/gorfc.go" "$WORK/sapmod/gorfc/gorfc.go"
cat > "$WORK/sapmod/go.mod" <<'EOF'
module github.com/sap/gorfc

go 1.21
EOF
cat > "$WORK/sap/go.mod" <<EOF
module sapside

go 1.21

require github.com/sap/gorfc v0.0.0

replace github.com/sap/gorfc => $WORK/sapmod
EOF
export CGO_CFLAGS="-I$SAPNWRFC_HOME/include"
export CGO_LDFLAGS="-L$SAPNWRFC_HOME/lib -lsapnwrfc -lsapucum"
export LD_LIBRARY_PATH="$SAPNWRFC_HOME/lib:${LD_LIBRARY_PATH:-}"
sed "s|GORFC_IMPORT_PATH|github.com/sap/gorfc/gorfc|" "$HERE/same_program.go.tmpl" > "$WORK/sap/main.go"
( cd "$WORK/sap" && go build -o program . )

# -- erpl-gorfc, against nothing at all ------------------------------------
echo "== building against erpl-gorfc ($BINDING)"
mkdir -p "$WORK/erpl"
cat > "$WORK/erpl/go.mod" <<EOF
module erplside

go 1.21

require github.com/DataZooDE/erpl-gorfc v0.0.0

replace github.com/DataZooDE/erpl-gorfc => $BINDING
EOF
# The asymmetry is the point: this side needs no CGO flags and no library path.
env -u CGO_CFLAGS -u CGO_LDFLAGS -u LD_LIBRARY_PATH \
  bash -c "cd '$WORK/erpl' && sed 's|GORFC_IMPORT_PATH|github.com/DataZooDE/erpl-gorfc/gorfc|' '$HERE/same_program.go.tmpl' > main.go && go build -o program ."

# -- run both, diff --------------------------------------------------------
echo "== running"
( cd "$WORK/sap" && LD_LIBRARY_PATH="$SAPNWRFC_HOME/lib" ./program ) > "$WORK/sap.txt" 2>&1 || true
( cd "$WORK/erpl" && env -u LD_LIBRARY_PATH ./program ) > "$WORK/erpl.txt" 2>&1 || true

if diff -u "$WORK/sap.txt" "$WORK/erpl.txt" > "$WORK/diff.txt"; then
  printf '\n\033[1mno differences across %s lines\033[0m\n' "$(wc -l < "$WORK/sap.txt")"
else
  echo
  cat "$WORK/diff.txt"
  echo
  echo "outputs: $WORK/sap.txt $WORK/erpl.txt"
  exit 1
fi

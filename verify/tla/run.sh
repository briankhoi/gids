#!/bin/sh
# Runs every TLC model in this directory and checks each against its
# expectation: *_main.cfg must find no error; *_neg_*.cfg (negative
# controls: the model deliberately broken) MUST find the violation, or the
# invariant it guards is proving nothing (VER-6, VER-15).
#
#   ./run.sh            (needs java and tla2tools.jar here; see README.md)
set -eu
cd "$(dirname "$0")"
[ -f tla2tools.jar ] || { echo "missing tla2tools.jar (see README.md)" >&2; exit 1; }
mkdir -p logs states
fail=0
for cfg in *.cfg; do
	name=${cfg%.cfg}
	spec=$(sed -n 's/^\\\* SPEC: *//p' "$cfg"); spec=${spec:-}
	case $name in
		files_*) spec=GidsFiles.tla ;;
		amend_*) spec=Amend.tla ;;
		guard_*) spec=Guard.tla ;;
		once_*)  spec=CoauthorOnce.tla ;;
		*) echo "?? $cfg: no spec for this prefix" >&2; fail=1; continue ;;
	esac
	log=logs/$name.log
	java -XX:+UseParallelGC -cp tla2tools.jar tlc2.TLC -workers auto -deadlock \
		-metadir "states/$name" -config "$cfg" "$spec" > "$log" 2>&1 || true
	states=$(sed -n 's/^\([0-9,]*\) states generated.*/\1/p' "$log" | tail -1)
	case $name in
	*_neg_*)
		if grep -q "^Error: Invariant .* is violated" "$log"; then
			echo "ok    $name: violation found as expected ($(grep -o 'Invariant [A-Za-z]* ' "$log" | head -1)), trace $(grep -c '^State [0-9]' "$log") states"
		else
			echo "FAIL  $name: expected a violation, got none"; fail=1
		fi ;;
	*)
		if grep -q "No error has been found" "$log"; then
			echo "ok    $name: no error, ${states:-?} states"
		else
			echo "FAIL  $name:"; grep -E "^Error|violated|deadlock|Parse|Semantic" "$log" | head -5; fail=1
		fi ;;
	esac
done
exit $fail

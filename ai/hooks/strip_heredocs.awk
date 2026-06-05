# strip_heredocs.awk — remove heredoc bodies from a shell command string.
#
# Used by safety_guard.sh so that text passed as a heredoc body (commit
# messages, doc strings, README content quoted into a script) doesn't trip
# the rm/curl-pipe/etc. regex rules.
#
# Recognized heredoc start syntax:
#   <<TAG          — interpolated
#   <<'TAG'        — literal
#   <<"TAG"        — literal
#   <<-TAG         — same as <<TAG, terminator may be tab-indented
#
# Terminator: a line equal to TAG (after stripping leading tabs for <<-).
#
# The heredoc start line itself is preserved (it still contains the executed
# command, e.g. `cat <<'EOF'`); only the body and terminator are dropped.

BEGIN { in_heredoc = 0; tag = "" }

in_heredoc {
    line = $0
    sub(/^\t+/, "", line)
    if (line == tag) {
        in_heredoc = 0
        tag = ""
    }
    next
}

{
    if (match($0, /<<-?[ \t]*["']?[A-Za-z_][A-Za-z0-9_]*["']?/)) {
        marker = substr($0, RSTART, RLENGTH)
        t = marker
        sub(/^<<-?[ \t]*["']?/, "", t)
        sub(/["']?$/, "", t)
        tag = t
        in_heredoc = 1
    }
    print
}

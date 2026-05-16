_tmuxinator() {
  local commands projects
  commands=(${(f)"$(tmuxinator commands zsh)"})
  projects=(${(f)"$(tmuxinator completions start)"})

  if (( CURRENT == 2 )); then
    _describe -t commands "tmuxinator subcommands" commands
    _describe -t projects "tmuxinator projects" projects
  elif (( CURRENT == 3)); then
    case $words[2] in
      copy|debug|delete|open|start)
      _arguments '*:projects:($projects)'
      ;;
    esac
  fi

  return
}
_muxhelp() {
  echo "
  Setup the gem with gem install tmuxinator
  See: https://github.com/tmuxinator/tmuxinator

  Use tmux

  https://gist.github.com/MohamedAlaa/2961058

  c  create window
  w  list windows
  n  next window
  p  previous window
  f  find window
  ,  name window
  &  kill window


  %  vertical split
  \"  horizontal split

  o  swap panes
  q  show pane numbers
  x  kill pane
  +  break pane into window (e.g. to select text by mouse to copy)
  -  restore pane from window
  ⍽  space - toggle between layouts
  prefix q Show pane numbers  when the numbers show up type the key to goto that pane
  prefix \{ Move the current pane left
  prefix \} Move the current pane right
  prefix z toggle pane zoom
"
}
export EDITOR='vim'
if command -v compdef &> /dev/null; then
    compdef _tmuxinator tmuxinator mux
fi
alias mux="tmuxinator"
alias muxhelp="_muxhelp"

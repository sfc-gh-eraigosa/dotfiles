"Autocommand goodness (still slightly beta :) ).
if has ("autocmd")
    " Show lines longer than 80 characters
    au BufWinEnter * let w:m1=matchadd('Search', '\%<80v.\%>77v', -1) 
    au BufWinEnter * let w:m2=matchadd('ErrorMsg', '\%>79v.\+', -1) 

    "Use default filetype settings. Overriding as necessary below.
    filetype plugin indent on

    "Autocommands to set up tab widths for c, c++ and java
    augroup C_code
        autocmd FileType c,cpp,java set shiftwidth=2 softtabstop=2
        autocmd FileType c,cpp,java set expandtab autoindent
    augroup END 

    "Autocommands to set up tab widths for python
    augroup python_code
        autocmd FileType python set shiftwidth=4 softtabstop=4
        autocmd FileType python set expandtab autoindent
    augroup END 

    "Autocommands to set the all important tab width for ruby. Also begins
    "new ruby files with the bang and saves it as executable.
    augroup ruby
        autocmd BufNewFile *.rb 0put ='#!/usr/bin/env ruby'
        autocmd FileType ruby set shiftwidth=2 softtabstop=2
        autocmd FileType ruby set expandtab autoindent
        autocmd BufWritePost *.rb !chmod 744 %
    augroup END 

    "Shell stuff sets autoindention details, the bang line, and saves the
    "file as an executable.
    augroup shell
        autocmd BufNewFile *.sh 0put ='#!/bin/sh'
        autocmd FileType sh set shiftwidth=4 softtabstop=4
        autocmd FileType sh set expandtab autoindent
        "autocmd BufWritePost *.sh !chmod 744 %
    augroup END 

    "When editing text documents I dont need the line numbers and text
    "should wrap around 80 characters.
    augroup text
        autocmd BufRead,BufNewFile *.txt set nonumber
        autocmd BufRead,BufNewFile *.txt set textwidth=80
    augroup END 
endif

" needs work
"source ~/.vimrc_vundle.vim

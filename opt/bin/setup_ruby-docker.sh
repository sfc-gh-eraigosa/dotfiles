function f-ruby-docker-setup {
    docker volume create ruby-bundle
}
function f-ruby-docker-run {
    docker run -it --rm -v ruby-bundle:/usr/local/bundle -v $(pwd):/workspace --workdir /workspace ruby $@
}
function f-ruby-docker-install {
    gem install rubocop
    gem install rspec
    gem install rails
}

alias ruby-docker-setup='f-ruby-docker-setup'
alias ruby-docker-install='f-ruby-docker-install'
for cmd in ruby rake bundler gem rails rubocop rspec; do
    eval 'alias $cmd="f-ruby-docker-run $cmd"'
done

# Reminder on setting up keys

Just some quick setup steps for gpg keys

1. Install gpg-suite `brew cask install gpg-suite`, then test it `gpg --version`
1. Generate the keys `gpg --full-generate-key`
1. List the setup keys `gpg --list-secret-keys --keyid-format LONG`
1. Configure the key `gpg --armor --export XXXXXXX`
1. Save the public key on GitHub
1. Configure git `git config --global user.signingkey XXXXXXXX`
1. Sign all commits `git config --global commit.gpgsign true`

Lots more details steps here:
- https://help.github.com/articles/generating-a-new-gpg-key/
- https://help.github.com/articles/adding-a-new-gpg-key-to-your-github-account/
- https://help.github.com/articles/signing-commits-using-gpg/

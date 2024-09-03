# Git cheat sheet for (busy) contributors

## Requirement: DCO and signing commits

Always DCO and sign your commits:

```bash
git -S -s -m "My commit"
```

Note: you do NOT need a gpg key to sign commits. You can just use your ssh key.

Edit your `~/.gitconfig`:

```
[user]
	name = me
	email = myemail@example.com
	signingkey = /myhome/.ssh/id_ed25519_sk.pub

[commit]
	gpgsign = true

[gpg]
	format = ssh
```

## Amend your commits when necessary instead of creating a bunch of new ones

You will probably have to squash anyhow, so, save yourself time and amend
when you are piling-up successive tweaks of the same thing.

```bash
git commit --amend
```

## Squash easy

Let say you have five commits that should really be one, because you did not `--amend`.

```bash
# This will rollback the last five commits, but keep local changes
git reset --soft HEAD~5
# Re-add everything
git add .
# Make one commit...
git commit -S -s -m "commit message"
```

## Rebasing against main

```bash
git checkout main
git pull
git checkout my-to-be-rebased-branch
git rebase main
```

## Add and commit only sections of a file

```bash
git add -p myfile
# ... confirm which segments you want to add
git commit -S -s -m "blabla"
```

### Manage and edit a pile of commits

Let say you have five commits for distincts things that should not be squashed.
And let say you want to delete an old one, edit a couple in between, and keep the rest as-is.

```bash
# You have some local changes ready for your modifications - stash them
git stash
# Rollback history to -5 commits
git rebase --interactive HEAD~5
```

Now:
```
####### for the commit you want to delete, comment out the line
# pick c5d3309d Add a locking mechanism to prevent concurrent subpackage testing
####### for the commit you want to modify, change `pick` to `edit`
edit 51af3690 Add Confirm helper
####### Leave the rest alone
pick b8b5d707 Move away flagutil under helpers
####### Edit this one two
edit 38bca1f3 Move completion to sub package
pick ec530147 Move constants Categroy/Management to helpers
```

Now, save and exit, you will drop into the first commit to edit.
Pop your stash, do modifications, amend the commit, then continue to the next commit:

```bash
git stash pop
git add somefile
git commit --amend
git rebase --continue
```

Repeat until you are all done rebasing everything.

## Bonus: name your branches in a consistent manner

Busy contributors will very fast end-up with a bunch of branches.
Using a scheme in the line of below will help you keep your sanity.

```bash
git branch bugfix-1234-some-bug-short-description
git branch feature-5678-some-feature-short-description
git branch refactor-9012-what
```

## Bonus: name your stashes

```bash
# Your future self will thank you
# Do not:
# git stash
# git stash list
# Do
git stash save -m "there is this thing in this stash"
git stash list
```


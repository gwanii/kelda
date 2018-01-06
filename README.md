# Kelda-Kube
This is a development fork for the transition from our custom scheduler to
Kubernetes.

## Repo Maintenance

**No branches are safe from force-pushing**. Only Kevin will force push.

### master
`master` will track the master branch of `kelda/kelda`.

### kube-stage
The `kube-stage` branch will track the "clean" version of the code. Only Kevin
will manage this branch. The goal of this branch is to have a working copy of
the Kubernetes implementation that we can always refer back to for checkpoints,
and as a way to organize commits that are ready to merge into the official
repo.

Every once in awhile, Kevin will merge commits from the `kube-dev` branch into this
branch. He might squash them to make the history cleaner. Commits that are ready
to be merged into the official repo will be based off this branch. Once merged,
this branch will be rebased against `master` of `kelda/kelda`.

### kube-dev
The `kube-dev` branch will be the shared development branch for both Kevin and Ethan.
This branch will be committed to much more frequently, maybe once a day. The
purpose of this branch is so that the code that Kevin and Ethan develop on
doesn't diverge too much. Once a set of commits have been moved to `kube-stage`,
Kevin will rebase this branch.

## TODOs

### Feature Complete
Missing features supported by the current scheduler implementation:
- Support environment variable values other than String
  - RuntimeInfo

### Kubernetes Robustness
Necessary Kubernetes improvements to the Kubernetes deployment to make it
reasonable to run in production.
- Test failover
  - Containers should continue running if the leader dies
  - Restarting a single leader should not result in downtime
  - Moving to secondary leader should not result in downtime
- Test rescheduling when placement rule changes (e.g. floating IP moves).

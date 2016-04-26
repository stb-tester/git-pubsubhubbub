git-pubsubhubbub
================

git-pubsubhubbub makes it possible to receive notification when a remote git
repo changes.  Subscribers use an HTTP POST to register a callback which will be
POSTed to by the git server once a change occurs.

It is:

* **Easy** to setup and deploy.  git-pubsubhubbub is portable and deployment
  consists of running a single, self-contained executable from any git repo.

* **Compatible** with the [github pubsubhubbub hooks][1].  If you integrate with
  git-pubsubhubbub you can reuse that exact same integration with github.  We're
  hoping to spur a standard way for clients to receive push notifications from
  git-repos.

* **Secure** - git-pubsubhubbub makes it easy to reuse the security mechanisms
  and policies you've already set-up around your git repos.  Anyone who has
  permission read from your git repo can register for callbacks, but no-one
  else can.  It takes care with the pubsubhubbub protocol to perform validation
  on requests and to properly sign notifications.

* **Decentralised** - You no-longer have to set-up your central git server to
  know about every client that could be interested in changes.  Clients register
  for interest using the same mechanism and via the same security policy as they
  pull changes - HTTP.  If you can poll for changes you can receive
  notifications too, so there's no longer an excuse to [poll][2].

[1]: https://developer.github.com/v3/repos/hooks/#pubsubhubbub
[2]: http://kohsuke.org/2011/12/01/polling-must-die-triggering-jenkins-builds-from-a-git-hook/

Advantages vs. Polling
----------------------

* Lower latency: subscribers are notified immediately on push and don't have to
  wait for the next polling interval to find out about a change.  This
  additional latency that polling requires can cause a considerable increase in
  the time it takes to get test results, particularly if your tests are fast and
  your polling interval is slow.

* Less resource consumption: Polling requires more wakeups and greater load on
  the clients, servers and networks.

Advantages vs. Custom Hooks
---------------------------

* You don't need to modify your git server configuration every time you add,
  update or remove a client.  Everyone who can poll a repo can register for
  callbacks.

* Simpler security: By applying the same security policy to git-pubsubhubbub
  as gitweb there's only one security policy to maintain and you don't need to
  modify and audit your manually set-up hooks every time you change your policy.

Quick Start
===========

1. Download git-pubsubhubbub for [Linux], [OS X] or [Windows]
2. Change to your git repo directory with `cd your-git-repo`
2. Run `git-pubsubhubbub`

You can now subscribe for changes on
http://localhost:8080/your-git-repo/events/push .

[Linux]: 
[OS X]: 
[Windows]: 

Usage
=====

    git-pubsubhubbub [-listen-address=LISTEN_ADDRESS] [-address-prefix=ADDRESS_PREFIX] [-projectroot=ROOT]

Security
========

Integration
===========

With gitweb
-----------

Licence
=======

Dual licence GPLv2 and 3-clause BSD.

The GPLv2 was chosen to make it easier to put into the upstream git project
proper.

The BSD licence was chosen to make it easier to split the pubsubhubbub hub bits
into its own go package to allow other people to reuse.  This is the same
licence that the go language/tooling is released under.

The BSD licence was also chosen to allow this to be as broadly distributed as
possible.  I'd like there to be one standard way of receiving change
notifications from a git repo over a network, so I want to remove any reason
someone would have to not run this software.

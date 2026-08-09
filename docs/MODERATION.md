# Moderation on Warpnet — a guide for everyone

Warpnet has no company, no instance admins and no moderation team. So who
takes down a death threat?

Short answer: **you report it, a handful of independent machines each judge it
with an AI model, and they vote.** No human sees your report, no one person can
force an outcome, and nobody can fake the result. This page explains how that
works, what you can expect from it, and — just as importantly — what it does
not promise.

---

## Reporting something

You can report **a post** or **a whole profile** (a profile gets judged on its
username, bio, website and other public profile text). Pick "Report", say in a
sentence what is wrong with it, and send.

That is the end of your involvement. There is no queue you have to follow, no
appeal form, no ticket number.

Two things are worth knowing before you press the button:

- **Direct messages are never moderated.** Nothing in your private chats is
  ever read, judged or reported — there is no mechanism for it at all. Only
  public posts and public profiles can be reported.
- **Your report is not anonymous to the moderators.** It carries your user ID,
  because that is how the answer finds its way back to you. The person you
  reported does not learn who reported them, but the machines that judge the
  report can see it.

---

## What happens next

### 1. Your report goes out to the moderators

Warpnet has volunteer **moderator nodes**: computers running the network's
moderation software with an AI model on board (currently Llama Guard 3). Anyone
can run one. Your report is broadcast to all of them at once — it is not sent
to a particular one, so nobody can position themselves to catch your report.

### 2. About three of them pick it up

If every moderator judged every report, the network would burn a lot of
electricity for nothing. Instead each one computes its own place in an
unpredictable order derived from the report itself, and only those at the front
start working. The rest wait a few seconds, see the answers already coming in,
and stay out of it.

Nobody assigns these judges and nobody can volunteer to judge a specific
report. The order comes out of the report's own contents, so an attacker cannot
arrange to be the one who reviews their friend's post.

### 3. Each one judges independently

Every judge fetches the reported post itself, runs it through its own copy of
the AI model, and forms its own opinion. They do not see each other's opinions
first, so they cannot copy each other.

### 4. They vote, and the majority wins

After a short window (about half a minute) the votes are counted. A post is
acted on only with a **strict majority against it**: two out of three, three
out of five, and so on. The count is always made odd — if an even number of
moderators voted, one vote is set aside by a rule every node computes the same
way, so a tie can never happen.

A tie or an even split always falls in favour of the content. So does a case
where the moderators disagree and no majority forms. Nothing gets actioned "by
default".

### 5. One of them announces the result

Only one moderator delivers the outcome, so you get exactly one answer rather
than three copies. Which one is decided by the same unpredictable ordering. If
that machine crashes or goes offline at that moment, the next one in line
notices the silence and steps in — a report does not get lost because one
computer died.

---

## What you will see

You get one notification, whatever the outcome:

| Outcome | What your notification says |
|---|---|
| The majority found a violation | *"The post you reported was moderated: <reason>"* |
| The majority found nothing wrong | *"The post you reported was reviewed: no violation found"* |
| The content could not be fetched | *"The post you reported could not be reviewed: the content is unavailable"* |

That last one usually means the author's node went offline before the
moderators could fetch the post. It is not a verdict, and nothing was decided.

Typically this takes well under a minute. If the author's node is slow or
briefly unreachable, the moderators retry a few times before giving up.

---

## What happens to moderated content

Warpnet uses a **shadow ban**, not a deletion:

- Everyone who follows the author stops seeing the offending post, and it drops
  out of their feeds.
- For a moderated profile, the profile text is hidden by the apps.
- **The author is never told.** Their own copy of their own post looks exactly
  as it did before.

This is deliberate. Someone who knows the exact moment they were caught simply
reposts with different wording; someone who does not know keeps shouting into a
room that quietly emptied. It also means nothing is deleted from anyone's
machine — Warpnet has no power to reach into your computer and erase files, and
that is by design.

The flip side is honest to state: **there is no appeal.** No human reviews the
outcome, and there is no button to contest it. The protection against a bad
call is that several independent machines had to agree, not that you can ask
someone to look again.

---

## Why a fake moderator cannot ban you

The obvious attack on a system like this is to pretend to be a moderator and
publish "verdicts" against people you dislike. That does not work here:

- **Every verdict is signed.** Each moderator has a cryptographic key that is
  built into its network identity. A verdict carries a signature that can only
  have been produced by the holder of that key.
- **Every app checks the signature before applying anything.** A verdict with a
  missing, broken or borrowed signature is discarded on the spot, before it can
  touch anything you see. Claiming to be someone else fails, because the
  signature will not match the identity being claimed.
- **One machine cannot decide alone.** Even a real moderator that turns
  malicious is only one vote out of an odd number, and it takes a strict
  majority to act.

There is a second layer, newly added: moderators **spot-check each other**.
Every few minutes a moderator picks another one at random and asks it to judge
a specific piece of content — one the network as a whole has already ruled on,
so the expected answer is known. A machine that answers at random, always says
the same thing, or has no AI model at all fails these checks and gets flagged
locally as unreliable.

Being honest about the limits of that last part: **those flags do not yet block
anyone.** They are recorded and logged, not enforced, because a check performed
by a single machine is still a single machine's opinion — and letting one node
disqualify another would create exactly the abuse it is meant to prevent.
Enforcement requires several independent checkers to agree first, and that is
not built yet.

---

## What this system does not promise

Worth reading, because these are real:

- **The AI can be wrong.** It is a model, not a judge. It will occasionally
  clear something it should have caught, and occasionally flag something
  harmless. Requiring a majority reduces this; it does not remove it.
- **Nothing is deleted.** A moderated post still exists on the author's machine
  and on anyone else's who already had it. Moderation hides content; it cannot
  erase it.
- **A verdict is applied by each app, not enforced by the network.** Warpnet is
  peer-to-peer: your app hides what the moderators condemned because it chooses
  to follow the protocol. Someone running modified software can ignore verdicts
  on their own screen. No decentralized system can prevent that, and any that
  claims otherwise is not being straight with you.
- **A determined attacker with many machines is still a problem.** Running a
  moderator costs nothing but a computer, so someone patient could stand up
  several and try to swing votes. This is the hardest unsolved problem in the
  design, and it is being worked on.
- **Coverage is only what is reported.** Nothing scans the network proactively.
  Content nobody reports is never reviewed.
- **No moderators online means no moderation.** Moderators are volunteers. If
  none are running, your report is accepted and then simply goes nowhere — you
  will not even get a notification. This was verified, and it is the honest
  failure mode of a network with no company behind it.

---

## Has any of this actually been tried?

Yes. Everything above was run on a private test network of real nodes — one
ordinary member node and three moderators, each with its own copy of the AI
model, talking to each other exactly as they would on the real network. Here is
what happened, including the awkward parts.

**The everyday cases**

| What was tried | What happened |
|---|---|
| Reported a post containing a direct threat | All three moderators judged it independently, all three voted against it, one announced the result. The post was hidden and the reporter was told it had been moderated — about 30 seconds end to end. |
| Reported an ordinary, harmless post | Judged clean by the majority. No hiding, nothing changed, and the reporter was told: *no violation found.* |
| Reported a post that no longer existed | The moderators tried to fetch it, retried, then reported honestly: *could not be reviewed.* Nothing was decided and nobody was penalised. |
| Reported the same post a second time | Nothing happened at all — the second report was recognised as the same case and did not trigger a new review or a second notification. |
| Reported a profile rather than a post | Reviewed the same way, with the profile hidden by the apps and the reporter notified. |

**The awkward cases**

| What was tried | What happened |
|---|---|
| Faked a verdict, claiming to be a real moderator | Rejected instantly: *signature invalid.* Nothing was hidden, nothing was stored. |
| Faked a verdict with a made-up moderator identity | Rejected just as fast: *no valid moderator id.* |
| Killed the announcing moderator mid-review | The next moderator in line noticed the silence, waited its ten seconds and delivered the verdict itself. The reporter still got exactly one answer, about ten seconds later than usual. |
| Left only two moderators running (an even number) | The tie-avoidance rule set one vote aside, the remaining verdict stood, and the moderator whose vote was set aside stayed on standby in case the announcer failed. |
| Left only one moderator running | It reviewed and decided alone. A lone moderator is weaker — nobody double-checks it — but reports do not pile up unanswered. |
| Took every moderator offline, then reported | The report was accepted by the app and then nothing happened: no review, no verdict, **and no notification**. If nobody is running a moderator, reports go nowhere. |

**The mutual spot-checks**

The moderators began checking each other automatically, without being asked.
Notably, they stayed quiet until the network had actually decided some cases —
with no settled examples to ask about, there is nothing to check anyone
against, and they correctly waited rather than inventing questions. Once
underway, every answer was correct and every moderator remained in the neutral
"still gathering evidence" state, which is the intended behaviour: judgement
needs a track record, not a single exchange.

**One result worth being honest about**

In one test the AI flagged a profile that contained nothing objectionable at
all. Nobody was harmed — this was a test network with test accounts — but it is
a concrete example of the caveat above: the model makes mistakes, in both
directions. The majority requirement is there to make a mistake need several
machines to agree on it, not to make mistakes impossible.

---

## Running a moderator yourself

Anyone can. A moderator node is the same software, started with the moderation
model loaded; it needs enough memory and CPU to run a small language model, and
a reasonably stable connection.

What it commits you to: your machine will fetch reported content and judge it,
it will answer spot-checks from other moderators, and its verdicts will carry
your node's signature. There is no application process and no approval — and no
special power either, since your vote counts the same as anyone else's and you
cannot pick which reports you judge.

If you want to help, see [`HOW-TO-HELP.md`](../HOW-TO-HELP.md); for the
technical side, the [Contributor Onboarding Guide](ONBOARDING.md) covers node
roles and how to build and run each one.

---

## In one paragraph

You report a post. The report reaches every moderator at once, and an
unpredictable few of them judge it independently with an AI model. They vote, a
strict majority is required to act, and the result comes back to you signed
with a key your app verifies before it applies anything. Moderated content is
hidden from other people's feeds, not deleted, and the author is never
notified. No human reads your report, no single machine can decide alone, and
nobody can forge a verdict.

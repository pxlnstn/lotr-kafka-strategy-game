# Game rules

A short, self-contained description of how the game works, so the code makes
sense without any other document. All of the values below come from the two
config files, `config/map.conf` and `config/units.conf`; the engine reads them
at startup and hardcodes nothing.

## The idea

Two human players share one map. The **Light** side (the Free Peoples) tries to
walk the Ring Bearer from the Shire to Mount Doom and destroy the Ring. The
**Dark** side (the Shadow) tries to find the Ring Bearer and catch him before he
gets there. There is no computer player; both sides are people, each in their
own browser.

The whole game turns on one asymmetry: the Light player always sees where the
Ring Bearer is, and the Dark player never does, unless a detection fires. Both
players see everything else - the map, every other unit, and every path's
status.

## The map

There are 22 regions joined by 37 one-way-or-the-other paths (every path can be
walked in both directions). Each path has a cost of 1 or 2, which is how many
turns of movement it represents.

Every region has:
- a **terrain** (plains, swamp, mountains, forest, fortress, or volcanic), which
  affects combat,
- a **controller** (Free Peoples, Shadow, or Neutral), which can change when a
  region is taken in battle,
- a **threat level**, used when scoring how dangerous a route is.

Two regions are special: the Shire is where the Ring Bearer starts, and Mount
Doom is the only place the Ring can be destroyed. There are also three Shadow
strongholds (Isengard, Minas Morgul, Mordor).

## The units

There are 13 units, 7 Light and 6 Dark. They all share one behaviour; what makes
them different is purely their configuration (strength, whether they lead, their
detection range, and so on).

Light: the Ring Bearer (Frodo), three Fellowship guards (Aragorn, Legolas,
Gimli), the Riders of Rohan, the Army of Gondor, and Gandalf.

Dark: the Witch-King and two lesser Nazgul (the Dark Marshal and the Betrayer),
the Uruk-hai Legion, Saruman, and Sauron.

A few traits matter:
- **Leadership** (Aragorn, the Witch-King): co-located allies of the same side
  fight at +1 strength.
- **Indestructible** (the Witch-King, Sauron): never destroyed; strength floors
  at 1 instead of dying.
- **Respawns** (the two lesser Nazgul): if destroyed, they come back at their
  home after a few turns.
- **Ignores fortress** (the Uruk-hai): when attacking, it ignores the defender's
  fortress terrain bonus.
- **Can fortify** (the Army of Gondor): can dig in to add a defensive bonus.
- **Maia** (Gandalf, Saruman, Sauron): each has one special ability, described
  below.
- **Detection range** (the three Nazgul): how far they can sense the Ring Bearer.

## A turn

Players submit orders during a turn; the turn then resolves in a fixed sequence:
routes are assigned, paths are blocked or searched, units are repositioned,
regions are fortified, Maia abilities fire, every unit auto-advances one step
along its route, battles are fought, timers tick down, respawns and cooldowns
count down, detection runs, and finally the win conditions are checked. Doing it
in that order keeps the result the same no matter who acted first.

## Movement and routes

A unit moves by being given a route: an ordered list of paths. Each turn it
takes one step along that route on its own. You can redirect a unit at any time
to give it a new route. The Ring Bearer moves exactly like everyone else, except
that its position is kept secret from the Dark side.

## Detection and the hidden Ring

For the first three turns nothing is detected at all - the Ring has a head start.
From turn four on, at the end of each turn the engine checks every Nazgul: if the
Ring Bearer is within that Nazgul's detection range (measured in path hops), the
Ring becomes "exposed" and the Dark side is told the region. The Ring is also
exposed if it crosses a path that the Dark side has put under surveillance.

Sauron adds to this. While he sits in Mordor, every Nazgul's detection range goes
up by one. He never moves and never takes orders; his effect is applied
automatically.

Being exposed does not by itself lose the game for Light - it only reveals the
position for that turn, and the flag resets at the end of the turn.

## Combat

When a unit attacks a region, the engine adds up the attackers' strengths and the
defenders' strengths, then adjusts the defenders:
- fortress terrain adds +2, mountains add +1 (unless the attacker ignores
  fortress terrain),
- an active fortification adds +2,
- leadership adds +1 to each co-located ally on a side that has a leader.

If the attackers' total is higher, the defenders take the difference as damage
and the region changes hands. Otherwise the attack is repelled and each attacker
loses one strength. An indestructible unit never drops below strength 1; a unit
that can respawn goes away and returns later; anyone else is destroyed. Taking
Isengard from the Shadow permanently disables Saruman.

## Paths: block, search, corrupt, open

A path has one of four states: OPEN, THREATENED, BLOCKED, or TEMPORARILY_OPEN,
plus a surveillance level from 0 to 3.

- **Block**: a unit standing at one end of a path can block it, which stops
  movement across it. A block only holds while that unit stays at the endpoint;
  if it leaves or is destroyed, the path opens again. A block also fails if an
  enemy unit is holding the other end - so a Fellowship guard parked at a path
  endpoint stops a Nazgul from sealing that path.
- **Search**: the Dark side can raise a path's surveillance, up to 3. A Ring
  Bearer crossing a surveilled path is exposed.
- **Open (Gandalf)**: Gandalf can re-open a blocked path for two turns.
- **Corrupt (Saruman)**: Saruman can permanently pin a path's surveillance to 3.

## The Maia and their abilities

All three Maia send the same kind of order, and the engine decides what it does
from each unit's configuration, not from its name:
- **Gandalf** has no listed paths, so his ability opens a blocked path for two
  turns.
- **Saruman** has a list of paths he is allowed to corrupt, so his ability
  corrupts one of them.
- **Sauron's** ability is passive (the detection boost above) and needs no order.

## Winning and losing

- **Light wins** if, at the end of a turn, the Ring Bearer is at Mount Doom, a
  Destroy Ring order was submitted that turn, and no Shadow unit is also at Mount
  Doom.
- **Dark wins** if a Nazgul is in the same region as the Ring Bearer and the Ring
  is exposed that turn.
- If 40 turns pass with no winner, the game is a draw.

## Where to look in the code

- The rules above are implemented in `option-b/internal/game` (combat, detection,
  movement, the path and unit state machines, the turn sequence, and the win
  check).
- The numbers (units, regions, paths, costs) live in `config/units.conf` and
  `config/map.conf`.

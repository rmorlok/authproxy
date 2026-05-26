import { useState } from 'react';

// Demo actor identities. The same three names are pre-created on the
// AuthProxy side by the umbrella chart's seed job (A11 / #300); each maps
// to a ConfiguredActor with namespace + permissions appropriate for the
// scenario the dropdown label describes.
const ACTORS: Array<{ id: string; label: string; description: string }> = [
  {
    id: 'demo-admin',
    label: 'Demo admin',
    description: 'Pre-configured admin actor with access to the admin UI.',
  },
  {
    id: 'demo-user',
    label: 'Demo user',
    description: 'Pre-configured end user with a working connection to the demo connector.',
  },
  {
    id: 'fresh-user',
    label: 'Fresh user',
    description: 'New user with no connections — lands on an empty marketplace.',
  },
];

const DESTINATIONS: Array<{ id: 'admin' | 'marketplace'; label: string }> = [
  { id: 'marketplace', label: 'Marketplace UI' },
  { id: 'admin', label: 'Admin UI' },
];

export function App() {
  const [actor, setActor] = useState(ACTORS[0]!.id);
  const [destination, setDestination] = useState<'admin' | 'marketplace'>('marketplace');

  const actorMeta = ACTORS.find((a) => a.id === actor)!;

  return (
    <div className="shell">
      <h1>AuthProxy Demo Shell</h1>
      <p className="lede">
        Stand-in host application. Pick an actor identity + a destination and the
        demo shell signs a JWT, redirects you in.
      </p>

      <form method="POST" action="/sso" className="card">
        <label>
          <span>Actor</span>
          <select name="actor" value={actor} onChange={(e) => setActor(e.target.value)}>
            {ACTORS.map((a) => (
              <option key={a.id} value={a.id}>
                {a.label}
              </option>
            ))}
          </select>
          <small>{actorMeta.description}</small>
        </label>

        <label>
          <span>Destination</span>
          <select
            name="destination"
            value={destination}
            onChange={(e) => setDestination(e.target.value as 'admin' | 'marketplace')}
          >
            {DESTINATIONS.map((d) => (
              <option key={d.id} value={d.id}>
                {d.label}
              </option>
            ))}
          </select>
        </label>

        <button type="submit">Sign in</button>
      </form>

      <footer>
        Demo environment only — never ship this shell to customers.
      </footer>
    </div>
  );
}

/* eslint-disable react/react-in-jsx-scope -- Vite uses the automatic JSX runtime. */
import { type FormEvent, useEffect, useMemo, useState } from 'react';

type Experience = 'admin' | 'marketplace' | 'telemetry' | 'oauth-provider';
type ActorID = 'demo-user' | 'fresh-user';

// The identities are pre-created by the demo seed job. Marketplace visitors
// choose one deliberately; the Admin UI always opens as demo-admin.
const MARKETPLACE_ACTORS: Array<{ id: ActorID; label: string; description: string }> = [
  {
    id: 'demo-user',
    label: 'Demo user',
    description: 'A pre-configured end user with a working demo connection to explore.',
  },
  {
    id: 'fresh-user',
    label: 'Fresh user',
    description: 'Start with an empty Marketplace and create a connection from scratch.',
  },
];

const EXPERIENCES: Array<{
  id: Experience;
  eyebrow: string;
  label: string;
  description: string;
  preview: string;
}> = [
  {
    id: 'admin',
    eyebrow: 'Manage AuthProxy',
    label: 'Admin UI',
    description: 'Configure namespaces, actors, connectors, tasks, encryption keys, and rate limits.',
    preview: 'admin',
  },
  {
    id: 'marketplace',
    eyebrow: 'Connect integrations',
    label: 'Integration Marketplace',
    description: 'Browse the seeded connector catalog and walk through an end-user connection flow.',
    preview: 'marketplace',
  },
  {
    id: 'telemetry',
    eyebrow: 'Observe the system',
    label: 'Telemetry',
    description: 'Review demo traffic, performance, traces, logs, and metrics in Grafana.',
    preview: 'telemetry',
  },
  {
    id: 'oauth-provider',
    eyebrow: 'Try a fake provider',
    label: 'Demo OAuth Provider',
    description: 'Open the disposable OAuth provider used by the Marketplace connector examples.',
    preview: 'oauth',
  },
];

type TelemetryLink = {
  label: string;
  description: string;
  url: string;
};

type ShellConfig = {
  oauthProviderUrl?: string;
  telemetryLinks?: TelemetryLink[];
};

function CredentialNotice() {
  return (
    <aside className="credential-notice" aria-label="Demo OAuth credentials">
      <p className="eyebrow">Fake OAuth account</p>
      <p>
        <strong>Username</strong> <code>demo-oauth-user@example.test</code>
        <br />
        <strong>Password</strong> <code>demo-password</code>
      </p>
      <p>
        You can also register a new account. This is a fake, disposable environment with no
        verification—never enter real credentials or data.
      </p>
    </aside>
  );
}

function ExperiencePreview({ kind }: { kind: string }) {
  return (
    <span className={`experience-preview preview-${kind}`} aria-hidden="true">
      <span />
      <span />
      <span />
    </span>
  );
}

export function App() {
  const [experience, setExperience] = useState<Experience | null>(null);
  const [actor, setActor] = useState<ActorID | null>(null);
  const [selectedTelemetryURL, setSelectedTelemetryURL] = useState<string | null>(null);
  const [telemetryLinks, setTelemetryLinks] = useState<TelemetryLink[]>([]);
  const [oauthProviderURL, setOAuthProviderURL] = useState<string | null>(null);

  const selectedTelemetry = useMemo(
    () => telemetryLinks.find((link) => link.url === selectedTelemetryURL) ?? null,
    [selectedTelemetryURL, telemetryLinks],
  );

  useEffect(() => {
    let cancelled = false;

    async function loadConfig() {
      try {
        const response = await fetch('/config.json');
        if (!response.ok) {
          return;
        }
        const config = (await response.json()) as ShellConfig;
        if (!cancelled) {
          setTelemetryLinks(config.telemetryLinks ?? []);
          setOAuthProviderURL(config.oauthProviderUrl ?? null);
        }
      } catch {
        if (!cancelled) {
          setTelemetryLinks([]);
          setOAuthProviderURL(null);
        }
      }
    }

    void loadConfig();

    return () => {
      cancelled = true;
    };
  }, []);

  function selectExperience(nextExperience: Experience) {
    setExperience(nextExperience);
    if (nextExperience !== 'marketplace') {
      setActor(null);
    }
    if (nextExperience !== 'telemetry') {
      setSelectedTelemetryURL(null);
    }
  }

  const canContinue =
    experience === 'admin' ||
    (experience === 'marketplace' && actor !== null) ||
    (experience === 'telemetry' && selectedTelemetry !== null) ||
    (experience === 'oauth-provider' && oauthProviderURL !== null);

  const submitLabel =
    experience === 'admin'
      ? 'Open Admin UI'
      : experience === 'marketplace'
        ? 'Open Marketplace'
        : experience === 'telemetry' && selectedTelemetry
          ? `Open ${selectedTelemetry.label}`
          : experience === 'oauth-provider'
            ? 'Open Demo OAuth Provider'
            : 'Go';

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    if (experience === 'telemetry' && selectedTelemetry) {
      event.preventDefault();
      window.location.assign(selectedTelemetry.url);
    }

    if (experience === 'oauth-provider' && oauthProviderURL) {
      event.preventDefault();
      window.location.assign(oauthProviderURL);
    }
  }

  return (
    <main className="shell">
      <header className="shell-header">
        <a className="wordmark" href="/" aria-label="AuthProxy Demo Shell home">
          AuthProxy <span>Demo</span>
        </a>
        <nav aria-label="AuthProxy resources">
          <a href="https://github.com/rmorlok/authproxy" target="_blank" rel="noreferrer">
            GitHub
          </a>
          <a href="https://docs.authproxy.net" target="_blank" rel="noreferrer">
            Docs
          </a>
          <a href="https://blog.authproxy.net" target="_blank" rel="noreferrer">
            Blog
          </a>
        </nav>
      </header>

      <section className="intro" aria-labelledby="demo-shell-heading">
        <p className="eyebrow">Interactive product tour</p>
        <h1 id="demo-shell-heading">Explore the AuthProxy demo</h1>
        <p>
          Choose the part of AuthProxy you want to try. The demo shell acts as a stand-in host
          application when a sign-in is needed.
        </p>
      </section>

      <form method="POST" action="/sso" className="wizard" onSubmit={handleSubmit}>
        {experience === 'admin' && (
          <>
            <input type="hidden" name="actor" value="demo-admin" />
            <input type="hidden" name="destination" value="admin" />
          </>
        )}
        {experience === 'marketplace' && actor && (
          <>
            <input type="hidden" name="actor" value={actor} />
            <input type="hidden" name="destination" value="marketplace" />
          </>
        )}

        <fieldset className="step">
          <legend>
            <span>Step 1</span> What would you like to explore?
          </legend>
          <div className="experience-grid">
            {EXPERIENCES.map((option) => (
              <label
                className={`experience-card ${experience === option.id ? 'is-selected' : ''}`}
                key={option.id}
              >
                <input
                  type="radio"
                  name="experience"
                  value={option.id}
                  checked={experience === option.id}
                  onChange={() => selectExperience(option.id)}
                />
                <ExperiencePreview kind={option.preview} />
                <span className="eyebrow">{option.eyebrow}</span>
                <span className="card-title">{option.label}</span>
                <span className="card-description">{option.description}</span>
              </label>
            ))}
          </div>
        </fieldset>

        {experience === 'admin' && (
          <section className="journey-summary" aria-live="polite">
            <p className="eyebrow">Ready to go</p>
            <h2>Sign in as the demo administrator</h2>
            <p>
              You’ll open the Admin UI as <code>demo-admin</code>, which has access to the full
              demo environment.
            </p>
          </section>
        )}

        {experience === 'marketplace' && (
          <section className="substep" aria-labelledby="marketplace-user-heading">
            <fieldset>
              <legend id="marketplace-user-heading">
                <span>Step 2</span> Who should connect to the Marketplace?
              </legend>
              <div className="actor-grid">
                {MARKETPLACE_ACTORS.map((option) => (
                  <label className={`actor-card ${actor === option.id ? 'is-selected' : ''}`} key={option.id}>
                    <input
                      type="radio"
                      name="actor-choice"
                      value={option.id}
                      checked={actor === option.id}
                      onChange={() => setActor(option.id)}
                    />
                    <span className="card-title">{option.label}</span>
                    <span className="card-description">{option.description}</span>
                  </label>
                ))}
              </div>
            </fieldset>
            <CredentialNotice />
          </section>
        )}

        {experience === 'telemetry' && (
          <section className="substep" aria-labelledby="telemetry-target-heading">
            <fieldset>
              <legend id="telemetry-target-heading">
                <span>Step 2</span> Which telemetry view would you like to open?
              </legend>
              {telemetryLinks.length > 0 ? (
                <div className="telemetry-grid">
                  {telemetryLinks.map((link) => (
                    <label
                      className={`telemetry-card ${selectedTelemetryURL === link.url ? 'is-selected' : ''}`}
                      key={link.url}
                    >
                      <input
                        type="radio"
                        name="telemetry"
                        value={link.url}
                        checked={selectedTelemetryURL === link.url}
                        onChange={() => setSelectedTelemetryURL(link.url)}
                      />
                      <span className="card-title">{link.label}</span>
                      <span className="card-description">{link.description}</span>
                    </label>
                  ))}
                </div>
              ) : (
                <p className="unavailable-message">
                  Telemetry is not configured in this demo environment yet.
                </p>
              )}
            </fieldset>
          </section>
        )}

        {experience === 'oauth-provider' && (
          <section className="substep oauth-journey" aria-live="polite">
            <div>
              <p className="eyebrow">Demo provider</p>
              <h2>Open the OAuth test environment</h2>
              <p>
                This is the fake provider behind the OAuth connector examples. Try signing in or
                register a new disposable account.
              </p>
              {!oauthProviderURL && (
                <p className="unavailable-message">
                  The OAuth provider is not configured in this environment yet.
                </p>
              )}
            </div>
            <CredentialNotice />
          </section>
        )}

        <button type="submit" className="go-button" disabled={!canContinue}>
          {submitLabel}
          <span aria-hidden="true">→</span>
        </button>
      </form>

      <footer>
        <p>Demo environment only — never ship this shell to customers.</p>
        <p>
          <a href="https://github.com/rmorlok/authproxy" target="_blank" rel="noreferrer">
            GitHub
          </a>
          <a href="https://docs.authproxy.net" target="_blank" rel="noreferrer">
            Documentation
          </a>
          <a href="https://blog.authproxy.net" target="_blank" rel="noreferrer">
            Blog
          </a>
        </p>
      </footer>
    </main>
  );
}

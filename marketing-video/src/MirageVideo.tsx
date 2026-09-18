import React from 'react';
import {
  AbsoluteFill,
  Audio,
  Easing,
  interpolate,
  Sequence,
  spring,
  staticFile,
  useCurrentFrame,
  useVideoConfig,
} from 'remotion';
import {loadFont as loadSora} from '@remotion/google-fonts/Sora';
import {loadFont as loadFigtree} from '@remotion/google-fonts/Figtree';
import {loadFont as loadJetBrains} from '@remotion/google-fonts/JetBrainsMono';
import {colors, DURATION_IN_FRAMES, FPS, HEIGHT, WIDTH} from './theme';

const {fontFamily: sora} = loadSora();
const {fontFamily: figtree} = loadFigtree();
const {fontFamily: mono} = loadJetBrains();

export {DURATION_IN_FRAMES, FPS, HEIGHT, WIDTH};

const ease = Easing.bezier(0.22, 1, 0.36, 1);

function useEnter(delay = 0, dur = 22) {
  const f = useCurrentFrame();
  const t = Math.max(0, f - delay);
  const o = interpolate(t, [0, dur], [0, 1], {
    extrapolateLeft: 'clamp',
    extrapolateRight: 'clamp',
    easing: ease,
  });
  const y = interpolate(t, [0, dur], [28, 0], {
    extrapolateLeft: 'clamp',
    extrapolateRight: 'clamp',
    easing: ease,
  });
  const s = interpolate(t, [0, dur], [0.96, 1], {
    extrapolateLeft: 'clamp',
    extrapolateRight: 'clamp',
    easing: ease,
  });
  return {opacity: o, transform: `translateY(${y}px) scale(${s})`};
}

function SoftBg({drift = 0}: {drift?: number}) {
  const frame = useCurrentFrame();
  const x1 = interpolate(frame + drift, [0, 900], [8, 22], {
    extrapolateRight: 'clamp',
  });
  const x2 = interpolate(frame + drift, [0, 900], [78, 62], {
    extrapolateRight: 'clamp',
  });
  const y1 = interpolate(frame + drift, [0, 900], [12, 28], {
    extrapolateRight: 'clamp',
  });
  return (
    <AbsoluteFill
      style={{
        background: `linear-gradient(145deg, ${colors.bg0} 0%, ${colors.bg1} 48%, ${colors.bg2} 100%)`,
      }}
    >
      <div
        style={{
          position: 'absolute',
          width: 720,
          height: 720,
          borderRadius: '50%',
          left: `${x1}%`,
          top: `${y1}%`,
          background:
            'radial-gradient(circle, rgba(11,124,134,0.16) 0%, rgba(11,124,134,0) 68%)',
          filter: 'blur(8px)',
        }}
      />
      <div
        style={{
          position: 'absolute',
          width: 640,
          height: 640,
          borderRadius: '50%',
          left: `${x2}%`,
          top: '48%',
          background:
            'radial-gradient(circle, rgba(232,120,58,0.12) 0%, rgba(232,120,58,0) 70%)',
          filter: 'blur(10px)',
        }}
      />
      <svg
        width={WIDTH}
        height={HEIGHT}
        style={{position: 'absolute', inset: 0, opacity: 0.35}}
      >
        {Array.from({length: 18}).map((_, i) => (
          <line
            key={i}
            x1={0}
            y1={60 + i * 58}
            x2={WIDTH}
            y2={20 + i * 58}
            stroke={colors.line}
            strokeWidth={1}
            opacity={0.25}
          />
        ))}
      </svg>
    </AbsoluteFill>
  );
}

function Cursor({
  x,
  y,
  clickAt,
}: {
  x: number;
  y: number;
  clickAt?: number;
}) {
  const frame = useCurrentFrame();
  const {fps} = useVideoConfig();
  const pulse =
    clickAt !== undefined
      ? spring({
          frame: frame - clickAt,
          fps,
          config: {damping: 14, stiffness: 160},
        })
      : 0;
  const scale = clickAt !== undefined ? 1 - pulse * 0.12 : 1;
  return (
    <div
      style={{
        position: 'absolute',
        left: x,
        top: y,
        transform: `scale(${scale})`,
        transformOrigin: '2px 2px',
        zIndex: 50,
        filter: 'drop-shadow(0 8px 16px rgba(18,32,43,0.25))',
      }}
    >
      <svg width="34" height="40" viewBox="0 0 24 28" fill="none">
        <path
          d="M3 2.5L3 22.5L8.2 17.8L12.1 26.1L15.2 24.7L11.2 16.2L18.5 15.8L3 2.5Z"
          fill={colors.ink}
          stroke={colors.white}
          strokeWidth="1.4"
          strokeLinejoin="round"
        />
      </svg>
      {clickAt !== undefined && frame >= clickAt && frame < clickAt + 18 ? (
        <div
          style={{
            position: 'absolute',
            left: -10,
            top: -10,
            width: 28,
            height: 28,
            borderRadius: '50%',
            border: `2px solid ${colors.brand}`,
            opacity: interpolate(frame - clickAt, [0, 18], [0.7, 0], {
              extrapolateRight: 'clamp',
            }),
            transform: `scale(${interpolate(frame - clickAt, [0, 18], [0.4, 1.8], {
              extrapolateRight: 'clamp',
            })})`,
          }}
        />
      ) : null}
    </div>
  );
}

function DrawnLine({
  x1,
  y1,
  x2,
  y2,
  delay = 0,
  color = colors.brand,
}: {
  x1: number;
  y1: number;
  x2: number;
  y2: number;
  delay?: number;
  color?: string;
}) {
  const frame = useCurrentFrame();
  const progress = interpolate(frame - delay, [0, 28], [0, 1], {
    extrapolateLeft: 'clamp',
    extrapolateRight: 'clamp',
    easing: ease,
  });
  const len = Math.hypot(x2 - x1, y2 - y1);
  return (
    <line
      x1={x1}
      y1={y1}
      x2={x2}
      y2={y2}
      stroke={color}
      strokeWidth={2.5}
      strokeLinecap="round"
      strokeDasharray={len}
      strokeDashoffset={len * (1 - progress)}
      opacity={0.85}
    />
  );
}

function Panel({
  children,
  style,
}: {
  children: React.ReactNode;
  style?: React.CSSProperties;
}) {
  return (
    <div
      style={{
        background: colors.panel,
        border: `1px solid ${colors.panelBorder}`,
        borderRadius: 20,
        boxShadow: colors.shadow,
        backdropFilter: 'blur(10px)',
        overflow: 'hidden',
        ...style,
      }}
    >
      {children}
    </div>
  );
}

function WindowChrome({title}: {title: string}) {
  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: 10,
        padding: '14px 18px',
        borderBottom: `1px solid ${colors.panelBorder}`,
        background: 'rgba(255,255,255,0.55)',
      }}
    >
      <div style={{display: 'flex', gap: 7}}>
        {['#F2A4A4', '#F2D27A', '#9AD4A8'].map((c) => (
          <div
            key={c}
            style={{width: 11, height: 11, borderRadius: '50%', background: c}}
          />
        ))}
      </div>
      <div
        style={{
          marginLeft: 8,
          fontFamily: figtree,
          fontSize: 15,
          color: colors.inkMuted,
          letterSpacing: 0.2,
        }}
      >
        {title}
      </div>
    </div>
  );
}

/* -------------------- Scenes -------------------- */

const Opening: React.FC = () => {
  const frame = useCurrentFrame();
  const {fps} = useVideoConfig();
  const brand = spring({frame, fps, config: {damping: 16, stiffness: 90}});
  const sub = useEnter(28, 26);
  const haze = interpolate(frame, [0, 90], [0, 1], {
    extrapolateRight: 'clamp',
  });
  return (
    <AbsoluteFill>
      <SoftBg />
      <AbsoluteFill
        style={{
          justifyContent: 'center',
          alignItems: 'center',
          transform: `scale(${interpolate(brand, [0, 1], [0.88, 1])})`,
          opacity: brand,
        }}
      >
        <div style={{textAlign: 'center', position: 'relative'}}>
          <div
            style={{
              position: 'absolute',
              inset: '-40px -80px',
              background: `radial-gradient(circle, rgba(11,124,134,${0.18 * haze}) 0%, transparent 70%)`,
            }}
          />
          <div
            style={{
              fontFamily: sora,
              fontWeight: 700,
              fontSize: 128,
              letterSpacing: -3,
              color: colors.ink,
              lineHeight: 1,
            }}
          >
            Mirage
          </div>
          <div style={sub}>
            <div
              style={{
                marginTop: 28,
                fontFamily: figtree,
                fontSize: 34,
                color: colors.inkMuted,
                maxWidth: 980,
                lineHeight: 1.35,
              }}
            >
              Kubernetes-native preview environments that appear for a pull
              request — and vanish when you&apos;re done.
            </div>
          </div>
        </div>
      </AbsoluteFill>
    </AbsoluteFill>
  );
};

const Problem: React.FC = () => {
  const frame = useCurrentFrame();
  const title = useEnter(0);
  const a = useEnter(18);
  const b = useEnter(42);
  const strikeA = interpolate(frame, [55, 75], [0, 1], {
    extrapolateLeft: 'clamp',
    extrapolateRight: 'clamp',
  });
  const strikeB = interpolate(frame, [95, 115], [0, 1], {
    extrapolateLeft: 'clamp',
    extrapolateRight: 'clamp',
  });
  return (
    <AbsoluteFill>
      <SoftBg drift={40} />
      <AbsoluteFill style={{padding: '120px 140px'}}>
        <div style={title}>
          <div
            style={{
              fontFamily: sora,
              fontWeight: 700,
              fontSize: 64,
              color: colors.ink,
              letterSpacing: -1.2,
              maxWidth: 1100,
              lineHeight: 1.15,
            }}
          >
            Every PR deserves a real environment.
          </div>
        </div>
        <div
          style={{
            marginTop: 72,
            display: 'flex',
            gap: 36,
            ...a,
          }}
        >
          {[
            {label: 'Shared staging cluster', strike: strikeA},
            {label: 'YAML archaeology', strike: strikeB},
          ].map((item) => (
            <Panel
              key={item.label}
              style={{
                padding: '36px 40px',
                minWidth: 420,
                position: 'relative',
              }}
            >
              <div
                style={{
                  fontFamily: figtree,
                  fontSize: 30,
                  color: colors.ink,
                  fontWeight: 600,
                }}
              >
                {item.label}
              </div>
              <div
                style={{
                  position: 'absolute',
                  left: 34,
                  right: 34,
                  top: '58%',
                  height: 4,
                  borderRadius: 4,
                  background: colors.danger,
                  transform: `scaleX(${item.strike})`,
                  transformOrigin: 'left center',
                  opacity: 0.85,
                }}
              />
            </Panel>
          ))}
        </div>
        <div style={{marginTop: 48, ...b}}>
          <div
            style={{
              fontFamily: figtree,
              fontSize: 26,
              color: colors.brandDeep,
              fontWeight: 600,
            }}
          >
            There&apos;s a cleaner way.
          </div>
        </div>
      </AbsoluteFill>
    </AbsoluteFill>
  );
};

const TemplateFlow: React.FC = () => {
  const frame = useCurrentFrame();
  const cam = interpolate(frame, [0, 280], [1.04, 1], {
    extrapolateRight: 'clamp',
    easing: ease,
  });
  const pan = interpolate(frame, [0, 280], [-18, 0], {
    extrapolateRight: 'clamp',
    easing: ease,
  });
  const cursorX = interpolate(frame, [40, 120, 180, 240], [520, 760, 1180, 1280], {
    extrapolateLeft: 'clamp',
    extrapolateRight: 'clamp',
    easing: ease,
  });
  const cursorY = interpolate(frame, [40, 120, 180, 240], [420, 360, 520, 560], {
    extrapolateLeft: 'clamp',
    extrapolateRight: 'clamp',
    easing: ease,
  });
  return (
    <AbsoluteFill>
      <SoftBg drift={80} />
      <AbsoluteFill
        style={{
          transform: `translate(${pan}px, 0px) scale(${cam})`,
          transformOrigin: '50% 45%',
        }}
      >
        <div style={{padding: '90px 120px'}}>
          <div
            style={{
              fontFamily: sora,
              fontWeight: 700,
              fontSize: 52,
              color: colors.ink,
              letterSpacing: -1,
              marginBottom: 36,
            }}
          >
            Platform defaults. Per-PR variance.
          </div>
          <div style={{display: 'flex', gap: 40, alignItems: 'stretch'}}>
            <Panel style={{width: 620}}>
              <WindowChrome title="PreviewTemplate · platform" />
              <pre
                style={{
                  margin: 0,
                  padding: 28,
                  fontFamily: mono,
                  fontSize: 20,
                  lineHeight: 1.55,
                  color: colors.ink,
                }}
              >
                <span style={{color: colors.brand}}>apiVersion</span>:
                mirage.dev/v1alpha1{'\n'}
                <span style={{color: colors.brand}}>kind</span>: PreviewTemplate
                {'\n'}
                metadata:{'\n'}
                {'  '}name: standard{'\n'}
                spec:{'\n'}
                {'  '}ttl: 48h{'\n'}
                {'  '}ingressDomain: previews.example.com{'\n'}
                {'  '}resources:{'\n'}
                {'    '}requests: {'{'}cpu: 100m{'}'}
              </pre>
            </Panel>
            <div
              style={{
                display: 'flex',
                flexDirection: 'column',
                justifyContent: 'center',
                alignItems: 'center',
                width: 80,
              }}
            >
              <svg width="80" height="220">
                <DrawnLine x1={10} y1={110} x2={70} y2={110} delay={50} />
              </svg>
            </div>
            <Panel style={{width: 680}}>
              <WindowChrome title="PreviewEnvironment · PR #142" />
              <pre
                style={{
                  margin: 0,
                  padding: 28,
                  fontFamily: mono,
                  fontSize: 20,
                  lineHeight: 1.55,
                  color: colors.ink,
                }}
              >
                <span style={{color: colors.brand}}>apiVersion</span>:
                mirage.dev/v1alpha1{'\n'}
                <span style={{color: colors.brand}}>kind</span>: PreviewEnvironment
                {'\n'}
                metadata:{'\n'}
                {'  '}name: pr-142{'\n'}
                spec:{'\n'}
                {'  '}templateRef: standard{'\n'}
                {'  '}image: ghcr.io/acme/api@sha256:…{'\n'}
                {'  '}pullRequest: &quot;142&quot;
              </pre>
            </Panel>
          </div>
        </div>
        <Cursor x={cursorX} y={cursorY} clickAt={185} />
      </AbsoluteFill>
    </AbsoluteFill>
  );
};

const Conditions: React.FC = () => {
  const frame = useCurrentFrame();
  // Timed to VO within Conditions scene starting ~28.2s
  const items = [
    {label: 'NamespaceReady', at: 140},
    {label: 'WorkloadReady', at: 198},
    {label: 'NetworkReady', at: 254},
    {label: 'RouteReady', at: 308},
  ];
  const urlReveal = interpolate(frame, [356, 386], [0, 1], {
    extrapolateLeft: 'clamp',
    extrapolateRight: 'clamp',
    easing: ease,
  });
  const cursorX = interpolate(frame, [380, 440], [980, 1120], {
    extrapolateLeft: 'clamp',
    extrapolateRight: 'clamp',
  });
  const cursorY = interpolate(frame, [380, 440], [620, 560], {
    extrapolateLeft: 'clamp',
    extrapolateRight: 'clamp',
  });
  return (
    <AbsoluteFill>
      <SoftBg drift={120} />
      <AbsoluteFill style={{padding: '100px 140px'}}>
        <div
          style={{
            fontFamily: sora,
            fontWeight: 700,
            fontSize: 52,
            color: colors.ink,
            marginBottom: 48,
          }}
        >
          Watch it come alive
        </div>
        <div style={{display: 'flex', flexDirection: 'column', gap: 18}}>
          {items.map((item, i) => {
            const on = frame >= item.at;
            const p = interpolate(frame - item.at, [0, 18], [0, 1], {
              extrapolateLeft: 'clamp',
              extrapolateRight: 'clamp',
              easing: ease,
            });
            return (
              <div
                key={item.label}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 20,
                  opacity: on ? p : 0.25,
                  transform: `translateX(${on ? (1 - p) * 24 : 0}px)`,
                }}
              >
                <div
                  style={{
                    width: 28,
                    height: 28,
                    borderRadius: '50%',
                    background: on ? colors.success : colors.line,
                    boxShadow: on
                      ? `0 0 0 ${8 * p}px rgba(31,154,107,0.18)`
                      : 'none',
                  }}
                />
                <div
                  style={{
                    fontFamily: mono,
                    fontSize: 30,
                    color: colors.ink,
                    fontWeight: 500,
                  }}
                >
                  {item.label}
                </div>
                {i < items.length - 1 ? (
                  <svg
                    width="40"
                    height="20"
                    style={{marginLeft: 8, opacity: 0.5}}
                  >
                    <DrawnLine
                      x1={0}
                      y1={10}
                      x2={40}
                      y2={10}
                      delay={item.at + 8}
                      color={colors.line}
                    />
                  </svg>
                ) : null}
              </div>
            );
          })}
        </div>
        <div
          style={{
            marginTop: 56,
            opacity: urlReveal,
            transform: `translateY(${(1 - urlReveal) * 20}px)`,
          }}
        >
          <Panel style={{width: 980, padding: 0}}>
            <WindowChrome title="preview · pr-142" />
            <div
              style={{
                padding: '22px 28px',
                display: 'flex',
                alignItems: 'center',
                gap: 16,
                background: colors.white,
              }}
            >
              <div
                style={{
                  flex: 1,
                  background: colors.bg1,
                  borderRadius: 12,
                  padding: '14px 18px',
                  fontFamily: mono,
                  fontSize: 22,
                  color: colors.brandDeep,
                }}
              >
                https://pr-142.previews.example.com
              </div>
              <div
                style={{
                  background: colors.brand,
                  color: colors.white,
                  fontFamily: figtree,
                  fontWeight: 700,
                  fontSize: 18,
                  padding: '14px 22px',
                  borderRadius: 12,
                }}
              >
                Open
              </div>
            </div>
          </Panel>
        </div>
        {frame > 375 ? <Cursor x={cursorX} y={cursorY} clickAt={428} /> : null}
      </AbsoluteFill>
    </AbsoluteFill>
  );
};

const MultiService: React.FC = () => {
  const frame = useCurrentFrame();
  const services = [
    {name: 'api', x: 220, y: 320, delay: 20},
    {name: 'frontend', x: 760, y: 250, delay: 40},
    {name: 'worker', x: 1280, y: 340, delay: 60},
  ];
  const deps = [
    {name: 'postgres', x: 430, y: 680, delay: 110},
    {name: 'redis', x: 980, y: 700, delay: 130},
  ];
  const cam = interpolate(frame, [0, 360], [0.97, 1.03], {
    extrapolateRight: 'clamp',
  });
  return (
    <AbsoluteFill>
      <SoftBg drift={160} />
      <AbsoluteFill
        style={{
          transform: `scale(${cam})`,
          transformOrigin: '50% 50%',
        }}
      >
        <div style={{padding: '80px 120px 0'}}>
          <div
            style={{
              fontFamily: sora,
              fontWeight: 700,
              fontSize: 48,
              color: colors.ink,
              marginBottom: 12,
            }}
          >
            Multi-service previews
          </div>
          <div
            style={{
              fontFamily: figtree,
              fontSize: 26,
              color: colors.inkMuted,
              marginBottom: 24,
            }}
          >
            API, frontend, worker — plus ephemeral Postgres & Redis
          </div>
        </div>
        <svg
          width={WIDTH}
          height={HEIGHT}
          style={{position: 'absolute', inset: 0, pointerEvents: 'none'}}
        >
          <DrawnLine x1={380} y1={400} x2={760} y2={340} delay={70} />
          <DrawnLine x1={980} y1={330} x2={1280} y2={400} delay={85} />
          <DrawnLine
            x1={380}
            y1={480}
            x2={500}
            y2={680}
            delay={140}
            color={colors.accent}
          />
          <DrawnLine
            x1={920}
            y1={360}
            x2={1040}
            y2={700}
            delay={155}
            color={colors.accent}
          />
        </svg>
        {[...services, ...deps].map((n) => {
          const p = interpolate(frame - n.delay, [0, 22], [0, 1], {
            extrapolateLeft: 'clamp',
            extrapolateRight: 'clamp',
            easing: ease,
          });
          const isDep = deps.some((d) => d.name === n.name);
          return (
            <div
              key={n.name}
              style={{
                position: 'absolute',
                left: n.x,
                top: n.y,
                opacity: p,
                transform: `translateY(${(1 - p) * 24}px) scale(${0.94 + 0.06 * p})`,
              }}
            >
              <Panel
                style={{
                  padding: '28px 34px',
                  minWidth: 240,
                  borderColor: isDep
                    ? 'rgba(232,120,58,0.28)'
                    : colors.panelBorder,
                  background: isDep
                    ? 'rgba(255,248,240,0.92)'
                    : colors.panel,
                }}
              >
                <div
                  style={{
                    fontFamily: mono,
                    fontSize: 14,
                    color: isDep ? colors.accent : colors.brand,
                    marginBottom: 8,
                    letterSpacing: 0.6,
                    textTransform: 'uppercase',
                  }}
                >
                  {isDep ? 'dependency' : 'service'}
                </div>
                <div
                  style={{
                    fontFamily: sora,
                    fontWeight: 700,
                    fontSize: 32,
                    color: colors.ink,
                  }}
                >
                  {n.name}
                </div>
                <div
                  style={{
                    marginTop: 10,
                    fontFamily: figtree,
                    fontSize: 16,
                    color: colors.inkSoft,
                  }}
                >
                  {isDep ? 'Secret-backed credentials' : 'Deployment + Service'}
                </div>
              </Panel>
            </div>
          );
        })}
      </AbsoluteFill>
    </AbsoluteFill>
  );
};

const CliDemo: React.FC = () => {
  const frame = useCurrentFrame();
  const lines = [
    {cmd: '$ mirage list', out: 'pr-142   Ready   https://pr-142.previews…', at: 85},
    {cmd: '$ mirage url pr-142', out: 'https://pr-142.previews.example.com', at: 148},
    {cmd: '$ mirage logs pr-142 --service frontend', out: 'listening on :8080', at: 208},
    {cmd: '$ mirage diagnose pr-142', out: 'All conditions Ready ✓', at: 268},
    {cmd: '$ mirage cost pr-142', out: 'est. $0.42 / day', at: 330},
  ];
  const camY = interpolate(frame, [0, 280], [20, -10], {
    extrapolateRight: 'clamp',
  });
  return (
    <AbsoluteFill>
      <SoftBg drift={200} />
      <AbsoluteFill
        style={{
          justifyContent: 'center',
          alignItems: 'center',
          transform: `translateY(${camY}px)`,
        }}
      >
        <div style={{width: 1180}}>
          <div
            style={{
              fontFamily: sora,
              fontWeight: 700,
              fontSize: 48,
              color: colors.ink,
              marginBottom: 28,
            }}
          >
            Manage with the Mirage CLI
          </div>
          <Panel>
            <WindowChrome title="mirage · terminal" />
            <div
              style={{
                background: '#102029',
                padding: '28px 32px',
                minHeight: 460,
                fontFamily: mono,
                fontSize: 22,
                lineHeight: 1.7,
              }}
            >
              {lines.map((l) => {
                const p = interpolate(frame - l.at, [0, 10], [0, 1], {
                  extrapolateLeft: 'clamp',
                  extrapolateRight: 'clamp',
                  easing: ease,
                });
                if (p <= 0.001) return null;
                return (
                  <div
                    key={l.cmd}
                    style={{
                      opacity: p,
                      transform: `translateY(${(1 - p) * 10}px)`,
                      marginBottom: 18,
                    }}
                  >
                    <div style={{color: '#7EE0C8'}}>{l.cmd}</div>
                    <div style={{color: '#D7E6EE'}}>{l.out}</div>
                  </div>
                );
              })}
              <span
                style={{
                  display: 'inline-block',
                  width: 12,
                  height: 24,
                  background: '#7EE0C8',
                  opacity: frame % 30 < 15 ? 1 : 0.2,
                  verticalAlign: 'middle',
                }}
              />
            </div>
          </Panel>
        </div>
      </AbsoluteFill>
    </AbsoluteFill>
  );
};

const Teardown: React.FC = () => {
  const frame = useCurrentFrame();
  const fade = interpolate(frame, [40, 140], [1, 0], {
    extrapolateLeft: 'clamp',
    extrapolateRight: 'clamp',
    easing: ease,
  });
  const words = [
    {t: 'Clean.', at: 155},
    {t: 'Isolated.', at: 195},
    {t: 'Gone.', at: 235},
  ];
  return (
    <AbsoluteFill>
      <SoftBg drift={240} />
      <AbsoluteFill style={{justifyContent: 'center', alignItems: 'center'}}>
        <div
          style={{
            opacity: fade,
            transform: `scale(${interpolate(fade, [0, 1], [0.92, 1])})`,
            textAlign: 'center',
          }}
        >
          <Panel style={{padding: '40px 56px', display: 'inline-block'}}>
            <div
              style={{
                fontFamily: mono,
                fontSize: 22,
                color: colors.inkMuted,
                marginBottom: 12,
              }}
            >
              preview / pr-142
            </div>
            <div
              style={{
                fontFamily: sora,
                fontWeight: 700,
                fontSize: 42,
                color: colors.ink,
              }}
            >
              PR closed · TTL expired
            </div>
          </Panel>
        </div>
        <div
          style={{
            position: 'absolute',
            display: 'flex',
            gap: 48,
            top: '55%',
          }}
        >
          {words.map((w) => {
            const p = interpolate(frame - w.at, [0, 18], [0, 1], {
              extrapolateLeft: 'clamp',
              extrapolateRight: 'clamp',
              easing: ease,
            });
            return (
              <div
                key={w.t}
                style={{
                  fontFamily: sora,
                  fontWeight: 700,
                  fontSize: 64,
                  color: colors.ink,
                  opacity: p,
                  transform: `translateY(${(1 - p) * 24}px)`,
                }}
              >
                {w.t}
              </div>
            );
          })}
        </div>
      </AbsoluteFill>
    </AbsoluteFill>
  );
};

const Closing: React.FC = () => {
  const frame = useCurrentFrame();
  const {fps} = useVideoConfig();
  const brand = spring({frame, fps, config: {damping: 15, stiffness: 80}});
  const tag = useEnter(28, 24);
  return (
    <AbsoluteFill>
      <SoftBg drift={280} />
      <AbsoluteFill
        style={{
          justifyContent: 'center',
          alignItems: 'center',
          opacity: brand,
          transform: `scale(${interpolate(brand, [0, 1], [0.94, 1])})`,
        }}
      >
        <div style={{textAlign: 'center'}}>
          <div
            style={{
              fontFamily: sora,
              fontWeight: 700,
              fontSize: 120,
              letterSpacing: -3,
              color: colors.ink,
            }}
          >
            Mirage
          </div>
          <div style={tag}>
            <div
              style={{
                marginTop: 24,
                fontFamily: figtree,
                fontSize: 36,
                color: colors.inkMuted,
              }}
            >
              Appears for a PR. Vanishes when done.
            </div>
            <div
              style={{
                marginTop: 40,
                display: 'inline-block',
                background: colors.brand,
                color: colors.white,
                fontFamily: figtree,
                fontWeight: 700,
                fontSize: 22,
                padding: '16px 28px',
                borderRadius: 14,
                boxShadow: '0 12px 30px rgba(11,124,134,0.28)',
              }}
            >
              github.com/sauravrana646/mirage
            </div>
          </div>
        </div>
      </AbsoluteFill>
    </AbsoluteFill>
  );
};

function Crossfade({
  children,
}: {
  children: React.ReactNode;
  durationInFrames?: number;
}) {
  const frame = useCurrentFrame();
  const fadeIn = interpolate(frame, [0, 14], [0, 1], {
    extrapolateRight: 'clamp',
    easing: ease,
  });
  return <AbsoluteFill style={{opacity: fadeIn}}>{children}</AbsoluteFill>;
}

export const MirageVideo: React.FC = () => {
  // Non-overlapping scenes (fade-in only) so layers never composite
  const scenes: {Comp: React.FC; from: number; dur: number}[] = [
    {Comp: Opening, from: 0, dur: 236}, // 0–7.9s
    {Comp: Problem, from: 236, dur: 250}, // 7.9–16.2s
    {Comp: TemplateFlow, from: 486, dur: 360}, // 16.2–28.2s
    {Comp: Conditions, from: 846, dur: 504}, // 28.2–45.0s
    {Comp: MultiService, from: 1350, dur: 510}, // 45–62s
    {Comp: CliDemo, from: 1860, dur: 390}, // 62–75s
    {Comp: Teardown, from: 2250, dur: 300}, // 75–85s
    {Comp: Closing, from: 2550, dur: 210}, // 85–92s
  ];

  return (
    <AbsoluteFill style={{backgroundColor: colors.bg0}}>
      <Audio src={staticFile('vo.mp3')} />
      {scenes.map(({Comp, from, dur}) => (
        <Sequence key={from} from={from} durationInFrames={dur} layout="none">
          <Crossfade>
            <Comp />
          </Crossfade>
        </Sequence>
      ))}
    </AbsoluteFill>
  );
};

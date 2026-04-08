import { useState, useEffect, useRef, useCallback } from 'react'
import './App.css'

type ViewState = 'game' | 'gallery' | 'leaderboard' | 'how-it-works' | 'buy-coins';
type Theme = 'light' | 'dark';

type PuzzleStatus = 'active' | 'solved';
export type Puzzle = { id: number, prompt: string | null, prize: number, winner: string | null, image: string, status: PuzzleStatus, tags: string[] };
export type AuthUser = { token: string, username: string } | null;

const viewToPath: Record<ViewState, string> = {
  'game': '/game',
  'gallery': '/gallery',
  'leaderboard': '/leaderboard',
  'how-it-works': '/how-it-works',
  'buy-coins': '/buy-coins',
};

const pathToView = Object.entries(viewToPath).reduce<Record<string, ViewState>>((acc, [view, path]) => {
  acc[path] = view as ViewState;
  return acc;
}, {} as Record<string, ViewState>);

const Header = ({ currentView, user, onLoginClick, onLogout, theme, onToggleTheme, onNavigate, selectedPuzzleId }: { currentView: ViewState, user: AuthUser, onLoginClick: () => void, onLogout: () => void, theme: Theme, onToggleTheme: () => void, onNavigate: (view: ViewState, puzzleId?: number) => void, selectedPuzzleId?: number }) => (
  <header className="fixed-header">
    <nav className="nav-container">
      <div className="logo-container">
        <div className="logo-icon">✨</div>
        <h1>
          <button type="button" style={{color:'inherit', textDecoration:'none', background:'transparent', border:'none', cursor:'pointer'}} onClick={() => onNavigate('game', selectedPuzzleId)}>
            Guess the Prompt
          </button>
        </h1>
      </div>
      <div className="nav-links">
        <button type="button" className={currentView === 'game' ? 'active' : ''} onClick={() => onNavigate('game', selectedPuzzleId)}>Game</button>
        <button type="button" className={currentView === 'gallery' ? 'active' : ''} onClick={() => onNavigate('gallery')}>Gallery</button>
        <button type="button" className={currentView === 'leaderboard' ? 'active' : ''} onClick={() => onNavigate('leaderboard')}>Leaderboard</button>
      </div>
      <div style={{display:'flex', alignItems:'center', gap:'1rem'}}>
        <button className="theme-toggle" onClick={onToggleTheme} title={`Switch to ${theme === 'light' ? 'dark' : 'light'} mode`}>
          {theme === 'light' ? '🌙' : '☀️'}
        </button>
        <div className="user-wealth">
          {user ? (
            <div style={{display:'flex', alignItems:'center', gap:'1rem'}}>
              <div style={{display:'flex', alignItems:'center', gap:'0.5rem'}}>
                 <span className="coin-icon">👤</span>
                 <span className="coin-amount" style={{fontSize: '1rem'}}>{user.username}</span>
              </div>
              <button className="secondary-btn" onClick={onLogout} style={{padding: '0.4rem 0.8rem', fontSize: '0.85rem'}}>Log Out</button>
            </div>
          ) : (
            <button className="primary-btn" onClick={onLoginClick} style={{padding: '0.5rem 1.5rem'}}>Login / Register</button>
          )}
        </div>
      </div>
    </nav>
  </header>
);

const AuthModal = ({ onClose, onAuthSuccess }: { onClose: () => void, onAuthSuccess: (u: AuthUser) => void }) => {
  const [isLogin, setIsLogin] = useState(true);
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!username || !password) return;
    setLoading(true);
    setError('');
    
    try {
      const endpoint = isLogin ? '/auth/login' : '/auth/register';
      const res = await fetch(`/v1${endpoint}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username, password })
      });
      if (!res.ok) {
        const text = await res.text();
        setError(text || 'Authentication failed');
        setLoading(false);
        return;
      }
      const data = await res.json();
      onAuthSuccess(data);
    } catch(err) {
      setError('Network error syncing with backend');
      setLoading(false);
    }
  };

  return (
    <div className="modal-overlay" style={{position:'fixed', top:0, left:0, right:0, bottom:0, background:'rgba(0,0,0,0.7)', zIndex: 1000, display:'flex', alignItems:'center', justifyContent:'center', backdropFilter:'blur(4px)'}}>
      <div className="panel" style={{width: '90%', maxWidth: '400px', padding: '2rem'}}>
        <div style={{display:'flex', justifyContent:'space-between', alignItems:'center', marginBottom:'1.5rem'}}>
          <h2 style={{margin:0}}>{isLogin ? 'Welcome Back' : 'Create Account'}</h2>
          <button onClick={onClose} style={{background:'transparent', border:'none', color:'var(--text-muted)', fontSize:'1.5rem', cursor:'pointer'}}>&times;</button>
        </div>
        
        {error && <div style={{background:'rgba(255,0,0,0.1)', color:'#ff6b6b', padding:'0.75rem', borderRadius:'8px', marginBottom:'1rem', fontSize:'0.9rem'}}>{error}</div>}
        
        <form onSubmit={handleSubmit} style={{display:'flex', flexDirection:'column', gap:'1rem'}}>
          <input type="text" placeholder="Username" className="gallery-search" value={username} onChange={e => setUsername(e.target.value)} required />
          <input type="password" placeholder="Password" className="gallery-search" value={password} onChange={e => setPassword(e.target.value)} required />
          <button type="submit" className="primary-btn" disabled={loading} style={{marginTop:'0.5rem', width: '100%'}}>
            {loading ? 'Authenticating...' : (isLogin ? 'Login' : 'Register')}
          </button>
        </form>
        
        <div style={{marginTop:'1.5rem', textAlign:'center', fontSize:'0.9rem', color:'var(--text-muted)'}}>
          {isLogin ? "Don't have an account? " : "Already have an account? "}
          <span onClick={() => setIsLogin(!isLogin)} style={{color:'var(--primary)', cursor:'pointer', fontWeight:'bold'}}>
            {isLogin ? 'Register' : 'Login'}
          </span>
        </div>
      </div>
    </div>
  )
}

const PuzzleSelector = ({ puzzles, selectedPuzzleId, onSelectPuzzle, onLoadMore, hasMore }: { puzzles: Puzzle[], selectedPuzzleId: number | null, onSelectPuzzle: (id: number) => void, onLoadMore: () => void, hasMore: boolean }) => {
  const scrollRef = useRef<HTMLDivElement>(null);
  const loadingRef = useRef(false);

  const handleScroll = () => {
    const el = scrollRef.current;
    if (!el || !hasMore || loadingRef.current) return;
    // Trigger load when within 200px of the right edge
    if (el.scrollLeft + el.clientWidth >= el.scrollWidth - 200) {
      loadingRef.current = true;
      onLoadMore();
      setTimeout(() => { loadingRef.current = false; }, 500);
    }
  };

  return (
    <div className="puzzle-selector-container">
      <div className="panel-header" style={{ borderBottom: 'none', paddingBottom: '0.5rem' }}>
        <h2 style={{ fontSize: '1rem' }}>Select a Puzzle</h2>
        <span style={{ color: 'var(--text-muted)', fontSize: '0.85rem' }}>Scroll to explore more puzzles</span>
      </div>
      <div className="puzzle-selector-scroll" ref={scrollRef} onScroll={handleScroll}>
        {puzzles.map(puzzle => (
          <div
            key={puzzle.id}
            className={`puzzle-thumbnail ${selectedPuzzleId === puzzle.id ? 'active' : ''}`}
            onClick={() => onSelectPuzzle(puzzle.id)}
          >
            <img src={puzzle.image} alt={`Puzzle #${puzzle.id}`} loading="lazy" />
            <div className="puzzle-thumbnail-overlay">
              <span className="puzzle-thumbnail-prize">💰 {puzzle.prize}</span>
              <span className={`puzzle-thumbnail-status ${puzzle.status}`}>
                {puzzle.status === 'active' ? '🟢 Active' : '🔒 Solved'}
              </span>
            </div>
            {puzzle.status === 'solved' && (
              <div className="puzzle-thumbnail-solved-badge">SOLVED</div>
            )}
          </div>
        ))}
        {hasMore && (
          <div style={{display:'flex', alignItems:'center', justifyContent:'center', minWidth:'80px', color:'var(--text-muted)', fontSize:'0.8rem'}}>
            Loading...
          </div>
        )}
      </div>
      {hasMore && (
        <div className="scroll-hint">
          <span className="scroll-hint-icon">→</span>
          <span>Swipe or scroll to see more</span>
        </div>
      )}
    </div>
  );
};

const ImageDisplay = ({ puzzle }: { puzzle: Puzzle }) => {
  console.log('[ImageDisplay] Puzzle ID:', puzzle.id, 'Image:', puzzle.image);
  return (
    <aside className="main-left-column panel">
      <div className="panel-header">
        <h2>AI Generation #{puzzle.id}</h2>
        <span className="badge">Active Puzzle</span>
      </div>
      <div className="image-container">
        <div className="image-wrapper">
          <img key={`${puzzle.id}-${puzzle.image}`} src={puzzle.image} alt="AI Generated Graphic" className="ai-image" />
          <div className="image-overlay">
            <div className="scanner"></div>
          </div>
        </div>
        <div className="puzzle-tags" style={{display:'flex', gap: '0.5rem', justifyContent:'center'}}>
          {puzzle.tags.map((tag: string) => <span key={tag} className="tag badge outline">#{tag}</span>)}
        </div>
        <p className="image-caption">Analyzing latent visual characteristics...</p>
      </div>
    </aside>
  );
};

type WsGuess = { puzzleId: number, username: string, words: string, matches: number, type: string };

const WordPuzzleArea = ({ puzzle, user, onLoginClick, onPrizeUpdate, lastWsGuess }: { puzzle: Puzzle, user: AuthUser, onLoginClick: () => void, onPrizeUpdate: (puzzleId: number, newPrize: number) => void, lastWsGuess: WsGuess | null }) => {
  const [promptText, setPromptText] = useState('');
  const [isFocused, setIsFocused] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [history, setHistory] = useState<any[]>([]);

  useEffect(() => {
    if (!user || !puzzle.id) return;
    fetch(`/v1/puzzles/${puzzle.id}/guesses`, {
      headers: { 'Authorization': `Bearer ${user.token}` }
    })
      .then(res => res.ok ? res.json() : [])
      .then(data => setHistory(data || []))
      .catch(() => {});
  }, [puzzle.id, user?.token]);

  useEffect(() => {
    if (!lastWsGuess || lastWsGuess.puzzleId !== puzzle.id) return;
    setHistory(prev => [{
      words: lastWsGuess.words,
      matches: lastWsGuess.matches,
      type: lastWsGuess.type,
      username: lastWsGuess.username,
    }, ...prev]);
  }, [lastWsGuess]);
  
  const prizePool = puzzle.prize;
  const platformFeePercentage = 30; // 30% platform
  const taxPercentage = 5; // Configurable mock tax
  const totalDeductions = platformFeePercentage + taxPercentage;
  const playerPrize = prizePool * (1 - (totalDeductions / 100));

  const words = promptText.trim().split(/\s+/).filter(w => w.length > 0);
  const coinCost = words.length; 
  
  const handleSubmit = async () => {
    if (!user) {
       onLoginClick();
       return;
    }

    if (!promptText.trim()) return;
    setIsSubmitting(true);
    
    try {
      const response = await fetch(`/v1/puzzles/${puzzle.id}/guess`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${user.token}`
        },
        body: JSON.stringify({ words: promptText })
      });
      
      if (!response.ok) {
         const errorText = await response.text();
         console.error(`[GuessPuzzle] ${response.status}: ${errorText}`);
         if (response.status === 429) {
             alert('Rate limit exceeded! Please slow down your guessing.');
         } else {
             alert(`Error ${response.status}: ${errorText}`);
         }
         setIsSubmitting(false);
         return;
      }
      
      const payload = await response.json();

      setHistory(prev => [{
        words: promptText,
        matches: payload.matches,
        type: payload.type
      }, ...prev]);
      
      if (payload.newPrizePool !== undefined) {
        onPrizeUpdate(puzzle.id, payload.newPrizePool);
      }
      
      setPromptText('');
      
      if (payload.success) {
         alert("Congratulations! You correctly guessed the entire prompt and claimed the Prize Pool!");
      }
    } catch(e) {
      console.error(e);
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <main className="main-right-column panel">
      <div className="panel-header">
        <h2>Your Guess</h2>
        <span className="badge outline">Cost: 1 Coin / Word</span>
      </div>
      
      <div className="prize-pool-hud">
        <div className="prize-top">
          <span className="prize-title">Current Prize Pool</span>
          <span className="prize-value">💰 {prizePool}</span>
        </div>
        <div className="prize-splits">
          <div className="split-item">Player Win: <span>💰 {playerPrize.toFixed(0)}</span> ({(100 - totalDeductions)}%)</div>
          <div className="split-item">Platform: <span>{platformFeePercentage}%</span></div>
          <div className="split-item">Tax Config: <span>{taxPercentage}%</span></div>
        </div>
      </div>

      <div className="guess-workspace">
        <div className={`textarea-wrapper ${isFocused ? 'active' : ''}`}>
          <textarea 
            className="guess-textarea"
            placeholder="Type words from the prompt... (e.g. neon cyberpunk golden hour)"
            value={promptText}
            onChange={(e) => setPromptText(e.target.value)}
            onFocus={() => setIsFocused(true)}
            onBlur={() => setIsFocused(false)}
            rows={4}
            disabled={isSubmitting}
          />
          <div className="textarea-footer">
            <span className="cost-warning">
              🪙 Cost to submit: {coinCost} Coin{coinCost !== 1 ? 's' : ''}
            </span>
            <button className="submit-btn" disabled={(!promptText.trim() && user !== null) || isSubmitting} onClick={handleSubmit}>
              {isSubmitting ? 'Validating...' : (!user ? 'Login to Guess' : 'Submit Guess')}
            </button>
          </div>
        </div>
        
        <div className="history-section">
          <h3>Guess Log</h3>
          <div className="history-list">
            {history.length === 0 && <p style={{color: 'var(--text-muted)', fontStyle: 'italic', fontSize: '0.9rem'}}>No guesses yet for this puzzle.</p>}
            {history.map((h, i) => (
              <div className="history-item" key={i}>
                <div className={`history-indicator ${h.type}`}></div>
                <div style={{flex: 1}}>
                  {h.username && <span style={{fontSize: '0.75rem', color: 'var(--text-muted)', marginBottom: '0.15rem', display: 'block'}}>{h.username}</span>}
                  <p>{h.words}</p>
                </div>
                <span className="similarity">{h.matches} Matches</span>
              </div>
            ))}
          </div>
        </div>
      </div>
    </main>
  );
};

const GalleryPage = ({ puzzles, onPlayPuzzle, onLoadMore, hasMore, total }: { puzzles: Puzzle[], onPlayPuzzle: (id: number) => void, onLoadMore: () => void, hasMore: boolean, total: number }) => {
  const [search, setSearch] = useState('');
  const [statusFilter, setStatusFilter] = useState<'all'|'active'|'solved'>('all');
  const [sortBy, setSortBy] = useState<'high-prize'|'low-prize'>('high-prize');
  const sentinelRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!hasMore || !sentinelRef.current) return;
    const observer = new IntersectionObserver((entries) => {
      if (entries[0].isIntersecting) onLoadMore();
    }, { rootMargin: '400px' });
    observer.observe(sentinelRef.current);
    return () => observer.disconnect();
  }, [hasMore, onLoadMore]);

  const filteredData = puzzles.filter(item => {
    if (statusFilter !== 'all' && item.status !== statusFilter) return false;
    if (search) {
      const q = search.toLowerCase();
      const matchTag = item.tags.some(t => t.toLowerCase().includes(q));
      const matchPrompt = item.prompt?.toLowerCase().includes(q);
      if (!matchTag && !matchPrompt) return false;
    }
    return true;
  }).sort((a, b) => {
    if (sortBy === 'low-prize') return a.prize - b.prize;
    return b.prize - a.prize;
  });

  return (
    <div className="page-container">
      <div className="panel-header" style={{ marginBottom: '1.5rem', borderBottom: 'none' }}>
        <h2>Puzzle Directory</h2>
        <p style={{ color: 'var(--text-muted)' }}>Showing {puzzles.length} of {total} puzzles. Explore successfully guessed artwork and discover active visual puzzles.</p>
      </div>

      <div className="gallery-controls panel" style={{marginBottom: '2rem', flexDirection: 'row', alignItems: 'center', flexWrap: 'wrap', gap: '1.5rem', padding: '1rem 1.5rem'}}>
        <input 
          type="text" 
          placeholder="Search tags or prompts..." 
          className="gallery-search" 
          value={search} 
          onChange={e => setSearch(e.target.value)} 
        />
        <div style={{display:'flex', gap:'1rem', flex: 1, justifyContent: 'flex-end'}}>
          <select className="gallery-select" value={statusFilter} onChange={e => setStatusFilter(e.target.value as any)}>
            <option value="all">All Puzzles</option>
            <option value="active">Active (Playable)</option>
            <option value="solved">Solved</option>
          </select>
          <select className="gallery-select" value={sortBy} onChange={e => setSortBy(e.target.value as any)}>
            <option value="high-prize">Highest Prize</option>
            <option value="low-prize">Lowest Prize</option>
          </select>
        </div>
      </div>

      <div className="gallery-grid">
        {filteredData.map(item => (
          <div className={`gallery-card ${item.status}`} key={item.id} onClick={() => item.status === 'active' && onPlayPuzzle(item.id)}>
            <div className="gallery-img-wrapper" style={{position:'relative'}}>
              <img src={item.image} alt="Art" loading="lazy" />
              {item.status === 'active' && (
                <div className="play-overlay">
                  <div className="play-btn-circle">▶</div>
                  <span>Play Now</span>
                </div>
              )}
            </div>
            <div className="gallery-info">
              {item.status === 'solved' ? (
                <>
                  <div className="gallery-prompt">"{item.prompt}"</div>
                  <div className="gallery-meta">
                    <span>Guessed by <strong>{item.winner}</strong></span>
                    <span>💰 {item.prize}</span>
                  </div>
                </>
              ) : (
                <div style={{display: 'flex', flexDirection: 'column', gap: '0.5rem', height: '100%'}}>
                  <div className="gallery-meta" style={{justifyContent: 'center', fontSize: '1.1rem', fontWeight: 700, color: 'var(--success)'}}>
                    💰 {item.prize} Prize Pool
                  </div>
                  <div className="gallery-meta" style={{justifyContent: 'center', gap: '0.5rem'}}>
                    {item.tags.slice(0,2).map(t => <span className="badge outline" style={{fontSize:'0.65rem'}} key={t}>{t}</span>)}
                    {item.tags.length > 2 && <span className="badge outline" style={{fontSize:'0.65rem'}}>+{item.tags.length - 2}</span>}
                  </div>
                </div>
              )}
            </div>
          </div>
        ))}
      </div>
      {hasMore && (
        <div ref={sentinelRef} style={{display:'flex', justifyContent:'center', padding:'2rem', color:'var(--text-muted)'}}>
          Loading more puzzles...
        </div>
      )}
    </div>
  );
};

const LeaderboardPage = () => {
  const [board, setBoard] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    fetch('/v1/leaderboard')
      .then(res => res.json())
      .then(data => {
        setBoard(data || []);
        setLoading(false);
      }).catch(err => {
        console.error(err);
        setLoading(false);
      });
  }, []);

  if (loading) return <div style={{textAlign:'center', marginTop:'4rem', color:'var(--text-main)'}}>Loading Global Rankings...</div>;
  if (!board.length) return <div style={{textAlign:'center', marginTop:'4rem', color:'var(--text-main)'}}>No rankings available yet.</div>;

  return (
    <div className="page-container leaderboard-container">
      <div className="panel-header" style={{ width: '100%', maxWidth: '900px', borderBottom: 'none', marginBottom: '0' }}>
        <h2>Top Prompt Breakers</h2>
        <p style={{ color: 'var(--text-muted)' }}>The highest earning players on the platform.</p>
      </div>
      
      {board.length >= 3 && (
        <div className="podium">
          <div className="podium-item podium-rank-2">
            <img src={board[1].avatar} className="podium-avatar" alt="2nd" />
            <div className="podium-block">
              <span className="podium-name">{board[1].name}</span>
              <span className="badge">#2</span>
            </div>
          </div>
          <div className="podium-item podium-rank-1">
            <img src={board[0].avatar} className="podium-avatar" alt="1st" />
            <div className="podium-block">
              <span className="podium-name">{board[0].name}</span>
              <span className="badge">#1</span>
            </div>
          </div>
          <div className="podium-item podium-rank-3">
            <img src={board[2].avatar} className="podium-avatar" alt="3rd" />
            <div className="podium-block">
              <span className="podium-name">{board[2].name}</span>
              <span className="badge">#3</span>
            </div>
          </div>
        </div>
      )}

      <div className="leaderboard-table-wrapper">
        <table className="leaderboard-table">
          <thead>
            <tr>
              <th>Rank</th>
              <th>Player</th>
              <th>Prompts Guessed</th>
              <th>Win Rate</th>
              <th>Total Earnings</th>
            </tr>
          </thead>
          <tbody>
            {board.map(player => (
              <tr key={player.rank}>
                <td>#{player.rank}</td>
                <td>
                  <div className="leaderboard-user">
                    <img src={player.avatar} alt={player.name} className="table-avatar" />
                    <strong>{player.name}</strong>
                  </div>
                </td>
                <td>{player.promptsGuessed}</td>
                <td>{player.winRate}</td>
                <td className="score-cell">💰 {player.coinsEarned.toLocaleString()}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
};

const FooterBar = ({ onNavigate }: { onNavigate: (view: ViewState) => void }) => (
  <footer className="fixed-footer">
    <div className="footer-content">
      <div className="status">
        <div className="pulse-dot"></div>
        <span>AI Engine Online &bull; Economy Active</span>
      </div>
      <div className="footer-actions">
        <button className="secondary-btn" onClick={() => onNavigate('how-it-works')}>How it Works</button>
        <button className="primary-btn" onClick={() => onNavigate('buy-coins')}>Buy Coins</button>
      </div>
    </div>
  </footer>
);

const HowItWorksPage = () => (
  <div className="page-container" style={{maxWidth: '800px', margin: '0 auto'}}>
    <div className="panel-header" style={{ borderBottom: '1px solid var(--border)', marginBottom: '2rem' }}>
      <h2>How to Play</h2>
      <p style={{ color: 'var(--text-muted)' }}>Master the prompt guessing economy</p>
    </div>
    <div className="instructions-list">
      <div className="instruction-step panel" style={{marginBottom: '1rem'}}>
        <h3>1. Analyze the Generated Artwork</h3>
        <p style={{marginTop: '0.5rem', color: 'var(--text-muted)'}}>Every round features a unique, AI-generated image. Your goal is to deduce the exact keywords used in the prompt that created it.</p>
      </div>
      <div className="instruction-step panel" style={{marginBottom: '1rem'}}>
        <h3>2. Submit Your Guesses</h3>
        <p style={{marginTop: '0.5rem', color: 'var(--text-muted)'}}>Type the words you believe were used in the original prompt. <strong>Punctuation and capitalization are completely ignored.</strong> You can guess words in any order!</p>
        <div className="warning-box" style={{background: 'rgba(255,165,0,0.1)', padding: '1rem', borderRadius: '8px', border: '1px solid rgba(255,165,0,0.3)', marginTop: '1rem', color: '#ffa500'}}>
          ⚠️ Each distinct word you submit costs <strong>1 GUESS Coin</strong>.
        </div>
      </div>
      <div className="instruction-step panel" style={{marginBottom: '1rem'}}>
        <h3>3. Win the Prize Pool</h3>
        <p style={{marginTop: '0.5rem', color: 'var(--text-muted)'}}>When someone correctly guesses all words in the original prompt, the round ends. The winner takes home the cumulative Prize Pool.</p>
        <ul style={{marginTop: '1rem', marginLeft: '1.5rem', color: 'var(--text-muted)'}}>
          <li><strong>Player Cut:</strong> 70% of the total pool</li>
          <li><strong>Platform Fee:</strong> 30%</li>
        </ul>
      </div>
    </div>
  </div>
);

const BuyCoinsPage = () => {
  const [selectedBundle, setSelectedBundle] = useState<number | null>(null);
  
  const handlePayment = (method: string) => {
    if (!selectedBundle) return;
    alert(`Mock Payment Triggered via ${method} for bundle #${selectedBundle}`);
  };

  return (
    <div className="page-container" style={{maxWidth: '1000px', margin: '0 auto'}}>
      <div className="panel-header" style={{ borderBottom: 'none', marginBottom: '2rem', textAlign: 'center' }}>
        <h2 style={{width:'100%', textAlign:'center'}}>Fund Your Wallet</h2>
        <p style={{ width:'100%', textAlign:'center', color: 'var(--text-muted)' }}>Purchase GUESS Coins to participate in high-stakes visual puzzles.</p>
      </div>
      <div className="pricing-grid">
        {[
          { id: 1, amount: 100, price: "$1.00", bonus: "0%" },
          { id: 2, amount: 500, price: "$4.00", bonus: "+20%" },
          { id: 3, amount: 1250, price: "$9.00", bonus: "+38%", popular: true },
          { id: 4, amount: 3000, price: "$20.00", bonus: "+50%" }
        ].map(b => (
          <div 
            key={b.id} 
            className={`pricing-card panel ${selectedBundle === b.id ? 'active' : ''} ${b.popular ? 'popular' : ''}`}
            onClick={() => setSelectedBundle(b.id)}
          >
            {b.popular && <div className="popular-badge">Most Popular</div>}
            <div className="coin-amount">💰 {b.amount}</div>
            <div className="bonus-pill">{b.bonus} Bonus</div>
            <div className="fiat-price">{b.price}</div>
          </div>
        ))}
      </div>
      {selectedBundle && (
        <div className="payment-actions panel" style={{ marginTop: '2rem', textAlign: 'center' }}>
          <h3 style={{marginBottom:'1rem'}}>Select Payment Method</h3>
          <div className="payment-buttons">
            <button className="stripe-btn" onClick={() => handlePayment('Stripe')}>Pay with Stripe</button>
            <button className="solana-btn" onClick={() => handlePayment('Solana')}>Pay with Solana</button>
          </div>
        </div>
      )}
    </div>
  );
};

function App() {
  const [currentView, setCurrentView] = useState<ViewState>('game');
  const [selectedPuzzleId, setSelectedPuzzleId] = useState<number | null>(null);
  const [puzzles, setPuzzles] = useState<Puzzle[]>([]);
  const [totalPuzzles, setTotalPuzzles] = useState(0);
  const [hasMorePuzzles, setHasMorePuzzles] = useState(false);
  const [isLoading, setIsLoading] = useState(true);
  const [theme, setTheme] = useState<Theme>(() => {
    const saved = localStorage.getItem('guess_theme');
    return (saved as Theme) || 'light';
  });
  
  const [user, setUser] = useState<AuthUser>(null);
  const [showAuthModal, setShowAuthModal] = useState(false);
  const [lastWsGuess, setLastWsGuess] = useState<{ puzzleId: number, username: string, words: string, matches: number, type: string } | null>(null);

  useEffect(() => {
    document.documentElement.setAttribute('data-theme', theme);
    localStorage.setItem('guess_theme', theme);
  }, [theme]);

  const toggleTheme = () => {
    setTheme(prev => prev === 'light' ? 'dark' : 'light');
  };

  useEffect(() => {
    // Attempt local storage hydration
    const savedUser = localStorage.getItem('guess_auth');
    if (savedUser) {
        try { setUser(JSON.parse(savedUser)); } catch (e) {}
    }

    const init = async () => {
      try {
        const res = await fetch('/v1/puzzles?limit=20&offset=0');
        const data = await res.json();
        const items: Puzzle[] = data.puzzles || [];
        const total: number = data.total || 0;
        console.log('[initial fetch] Got', items.length, 'of', total, 'puzzles');

        setPuzzles(items);
        setTotalPuzzles(total);
        setHasMorePuzzles(items.length < total);

        if (items.length > 0) {
          const pathname = window.location.pathname;
          const gameMatch = pathname.match(/^\/game\/(\d+)$/);
          const targetId = gameMatch ? parseInt(gameMatch[1], 10) : items[0].id;
          let targetPuzzle = items.find((p: Puzzle) => p.id === targetId);

          // If the target puzzle wasn't in the first page, fetch it directly
          if (!targetPuzzle && gameMatch) {
            try {
              const singleRes = await fetch(`/v1/puzzles/${targetId}`);
              if (singleRes.ok) {
                const single: Puzzle = await singleRes.json();
                setPuzzles(prev => {
                  if (prev.find(p => p.id === single.id)) return prev;
                  return [single, ...prev];
                });
                targetPuzzle = single;
              }
            } catch {}
          }

          setSelectedPuzzleId(targetPuzzle ? targetPuzzle.id : items[0].id);
        }
      } catch (err) {
        console.error("Failed to load puzzles from API:", err);
      } finally {
        setIsLoading(false);
      }
    };
    init();
  }, []);

  const navigateTo = (view: ViewState, puzzleId?: number, replace: boolean = false) => {
    setCurrentView(view);
    let path = viewToPath[view] || viewToPath.game;
    if (view === 'game' && puzzleId) {
      path = `/game/${puzzleId}`;
    }
    const state = { view, puzzleId };
    if (replace) {
      window.history.replaceState(state, '', path);
    } else {
      window.history.pushState(state, '', path);
    }
  };

  useEffect(() => {
    const syncFromPath = (replaceIfNeeded = true) => {
      const pathname = window.location.pathname === '/' ? '/game' : window.location.pathname;
      
      // Check for /game/:id pattern
      const gameMatch = pathname.match(/^\/game\/(\d+)$/);
      if (gameMatch) {
        const puzzleId = parseInt(gameMatch[1], 10);
        setCurrentView('game');
        setSelectedPuzzleId(puzzleId);
        if (replaceIfNeeded) {
          window.history.replaceState({ view: 'game', puzzleId }, '', pathname);
        }
        return;
      }
      
      const view = pathToView[pathname] || 'game';
      setCurrentView(view);
      if (replaceIfNeeded) {
        const desiredPath = viewToPath[view];
        if (pathname !== desiredPath) {
          window.history.replaceState({ view }, '', desiredPath);
        } else {
          window.history.replaceState({ view }, '', pathname);
        }
      }
    };

    const handlePopState = (event: PopStateEvent) => {
      if (event.state?.view) {
        setCurrentView(event.state.view as ViewState);
        if (event.state.puzzleId) {
          setSelectedPuzzleId(event.state.puzzleId);
        }
      } else {
        syncFromPath(false);
      }
    };

    syncFromPath(true);
    window.addEventListener('popstate', handlePopState);
    return () => window.removeEventListener('popstate', handlePopState);
  }, []);

  const refreshPuzzles = async () => {
    try {
      const res = await fetch('/v1/puzzles?limit=20&offset=0');
      const data = await res.json();
      const items: Puzzle[] = data.puzzles || [];
      const total: number = data.total || 0;
      console.log('[refreshPuzzles] Got', items.length, 'of', total, 'puzzles');
      setPuzzles(items);
      setTotalPuzzles(total);
      setHasMorePuzzles(items.length < total);
    } catch (err) {
      console.error("Failed to refresh puzzles:", err);
    }
  };

  const loadMorePuzzles = useCallback(async () => {
    if (!hasMorePuzzles) return;
    const currentLen = puzzles.length;
    try {
      const res = await fetch(`/v1/puzzles?limit=20&offset=${currentLen}`);
      const data = await res.json();
      const items: Puzzle[] = data.puzzles || [];
      const total: number = data.total || 0;
      setPuzzles(prev => {
        const existingIds = new Set(prev.map(p => p.id));
        const newItems = items.filter(p => !existingIds.has(p.id));
        return [...prev, ...newItems];
      });
      setTotalPuzzles(total);
      setHasMorePuzzles(currentLen + items.length < total);
    } catch (err) {
      console.error("Failed to load more puzzles:", err);
    }
  }, [hasMorePuzzles, puzzles.length]);

  const handleAuthSuccess = (u: AuthUser) => {
    setUser(u);
    localStorage.setItem('guess_auth', JSON.stringify(u));
    setShowAuthModal(false);
  };

  const handleLogout = () => {
    setUser(null);
    localStorage.removeItem('guess_auth');
  }

  // WebSocket for real-time prize pool updates from other players
  useEffect(() => {
    const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const ws = new WebSocket(`${wsProtocol}//${window.location.host}/ws`);
    ws.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data);
        if (msg.type === 'prize_update' && msg.puzzleId && msg.prize !== undefined) {
          setPuzzles(prev => prev.map(p => p.id === msg.puzzleId ? { ...p, prize: msg.prize } : p));
        }
        if (msg.type === 'new_guess' && msg.puzzleId) {
          setLastWsGuess({
            puzzleId: msg.puzzleId,
            username: msg.username || 'Anonymous',
            words: msg.words || '',
            matches: msg.matches || 0,
            type: msg.matches > 0 ? 'partial' : 'wrong',
          });
        }
      } catch (e) {
        console.error('[ws] parse error:', e);
      }
    };
    ws.onclose = () => console.log('[ws] disconnected');
    ws.onerror = (e) => console.error('[ws] error:', e);
    return () => ws.close();
  }, []);

  const handlePrizeUpdate = (puzzleId: number, newPrize: number) => {
    setPuzzles(prev => prev.map(p => p.id === puzzleId ? { ...p, prize: newPrize } : p));
  };

  const handlePlayPuzzle = (id: number) => {
    setSelectedPuzzleId(id);
    navigateTo('game', id);
    // Refresh after a short delay to ensure image is loaded
    setTimeout(refreshPuzzles, 500);
  };

  if (isLoading) {
    return <div style={{display: 'flex', height: '100vh', alignItems: 'center', justifyContent: 'center', color: 'var(--text-main)'}}>Connecting to AI Engine...</div>;
  }

  const currentPuzzleData = puzzles.find(p => p.id === selectedPuzzleId) || puzzles[0];

  return (
    <div className="app-layout">
      {showAuthModal && <AuthModal onClose={() => setShowAuthModal(false)} onAuthSuccess={handleAuthSuccess} />}
      
      <Header
        currentView={currentView}
        user={user}
        onLoginClick={() => setShowAuthModal(true)}
        onLogout={handleLogout}
        theme={theme}
        onToggleTheme={toggleTheme}
        onNavigate={navigateTo}
        selectedPuzzleId={selectedPuzzleId || undefined}
      />
      
      {currentView === 'game' && currentPuzzleData && (
        <div className="game-layout">
          <PuzzleSelector 
            puzzles={puzzles} 
            selectedPuzzleId={selectedPuzzleId} 
            onSelectPuzzle={(id) => {
              setSelectedPuzzleId(id);
              navigateTo('game', id, true);
            }}
            onLoadMore={loadMorePuzzles}
            hasMore={hasMorePuzzles}
          />
          <div className="game-main-content">
            <ImageDisplay puzzle={currentPuzzleData} />
            <WordPuzzleArea puzzle={currentPuzzleData} user={user} onLoginClick={() => setShowAuthModal(true)} onPrizeUpdate={handlePrizeUpdate} lastWsGuess={lastWsGuess} />
          </div>
        </div>
      )}
      
      {currentView === 'gallery' && <GalleryPage puzzles={puzzles} onPlayPuzzle={handlePlayPuzzle} onLoadMore={loadMorePuzzles} hasMore={hasMorePuzzles} total={totalPuzzles} />}
      {currentView === 'leaderboard' && <LeaderboardPage />}
      {currentView === 'how-it-works' && <HowItWorksPage />}
      {currentView === 'buy-coins' && <BuyCoinsPage />}
      
      <FooterBar onNavigate={navigateTo} />
    </div>
  )
}

export default App

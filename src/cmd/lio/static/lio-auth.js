// lio-auth.js — account/auth wiring for the header controls on every page:
// the login/register modal tabs + fetch submission (logged out), the login-time
// second factor (TOTP / recovery code / passkey), the profile popover actions,
// and the two-factor & passkey management modal (logged in). Server endpoints
// live under /api/auth (arch/ACCOUNTS_AUTH_RATINGS.md). Loaded deferred, so the
// DOM is parsed by the time this runs.
(() => {
	'use strict';

	/**
	 * POST JSON to an /api/auth endpoint.
	 * @param {string} path
	 * @param {object|null} body
	 * @returns {Promise<{status: number, data: object}>}
	 */
	const post = async (path, body) => {
		const res = await fetch(path, {
			method: 'POST',
			headers: body ? { 'Content-Type': 'application/json' } : {},
			body: body ? JSON.stringify(body) : undefined,
		});
		let data = {};
		try { data = await res.json(); } catch (e) { /* 204 etc. */ }
		return { status: res.status, data };
	};

	/**
	 * Show an error line inside a form/container (or hide it when msg is falsy).
	 * @param {Element} scope
	 * @param {string} [msg]
	 */
	const showError = (scope, msg) => {
		const el = scope && scope.querySelector('[data-auth-error]');
		if (!el) { return; }
		el.textContent = msg || '';
		el.classList.toggle('hidden', !msg);
	};

	const esc = (s) => String(s == null ? '' : s).replace(/[&<>"']/g, (c) => (
		{ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));

	// --- WebAuthn helpers ---------------------------------------------------

	const webAuthnSupported = () =>
		typeof window.PublicKeyCredential !== 'undefined' &&
		!!(navigator.credentials && navigator.credentials.create);

	// base64url <-> ArrayBuffer. go-webauthn encodes binary fields (challenge,
	// user.id, credential ids) as unpadded base64url; the browser needs
	// BufferSources going in and returns ArrayBuffers we re-encode going out.
	const b64urlToBuf = (s) => {
		s = s.replace(/-/g, '+').replace(/_/g, '/');
		const pad = s.length % 4;
		if (pad) { s += '='.repeat(4 - pad); }
		const bin = atob(s);
		const buf = new Uint8Array(bin.length);
		for (let i = 0; i < bin.length; i++) { buf[i] = bin.charCodeAt(i); }
		return buf.buffer;
	};
	const bufToB64url = (buf) => {
		const bytes = new Uint8Array(buf);
		let bin = '';
		for (let i = 0; i < bytes.length; i++) { bin += String.fromCharCode(bytes[i]); }
		return btoa(bin).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
	};

	// Decode server creation options for navigator.credentials.create.
	const prepCreation = (pk) => {
		pk.challenge = b64urlToBuf(pk.challenge);
		pk.user.id = b64urlToBuf(pk.user.id);
		if (pk.excludeCredentials) {
			pk.excludeCredentials = pk.excludeCredentials.map((c) => ({ ...c, id: b64urlToBuf(c.id) }));
		}
		return pk;
	};
	// Decode server assertion options for navigator.credentials.get.
	const prepAssertion = (pk) => {
		pk.challenge = b64urlToBuf(pk.challenge);
		if (pk.allowCredentials) {
			pk.allowCredentials = pk.allowCredentials.map((c) => ({ ...c, id: b64urlToBuf(c.id) }));
		}
		return pk;
	};

	// Serialize a PublicKeyCredential (attestation or assertion) into the JSON
	// shape go-webauthn's parser expects.
	const credentialToJSON = (cred) => {
		const r = cred.response;
		const out = {
			id: cred.id,
			type: cred.type,
			rawId: bufToB64url(cred.rawId),
			clientExtensionResults: cred.getClientExtensionResults ? cred.getClientExtensionResults() : {},
			response: { clientDataJSON: bufToB64url(r.clientDataJSON) },
		};
		if (r.attestationObject) {
			out.response.attestationObject = bufToB64url(r.attestationObject);
		} else {
			out.response.authenticatorData = bufToB64url(r.authenticatorData);
			out.response.signature = bufToB64url(r.signature);
			out.response.userHandle = r.userHandle ? bufToB64url(r.userHandle) : null;
		}
		return out;
	};

	// --- logged-in: profile popover actions --------------------------------
	const logoutBtn = document.getElementById('logoutButton');
	if (logoutBtn) {
		logoutBtn.addEventListener('click', async () => {
			logoutBtn.disabled = true;
			try { await post('/api/auth/logout', null); } catch (e) { /* still reload */ }
			window.location.reload();
		});
	}

	// log out everywhere: revoke all sessions (incl. this one) then reload
	const logoutAllBtn = document.getElementById('logoutAllButton');
	if (logoutAllBtn) {
		logoutAllBtn.addEventListener('click', async () => {
			logoutAllBtn.disabled = true;
			try { await post('/api/auth/logout-all', null); } catch (e) { /* still reload */ }
			window.location.reload();
		});
	}

	// ratings summary: lazy-load per-category Glicko-2 ratings into the profile
	// popover the first time it is opened (no per-page-render DB read)
	const ratingsSummary = document.getElementById('ratingsSummary');
	const profileBtn = document.getElementById('profileButton');
	if (ratingsSummary && profileBtn) {
		profileBtn.addEventListener('click', async () => {
			if (ratingsSummary.dataset.loaded === 'true') { return; }
			ratingsSummary.dataset.loaded = 'true';
			try {
				const res = await fetch('/api/auth/ratings');
				if (!res.ok) { throw new Error('ratings'); }
				const rows = (await res.json()).ratings || [];
				if (!rows.length) {
					ratingsSummary.innerHTML = '<p class="auth-hint">No rated games yet.</p>';
					return;
				}
				// rows arrive sorted (default mode first, then bullet<blitz<rapid).
				// Group by game mode into uniform rectangular cards; a mode header
				// renders only for a non-default mode — the default deploy mode is
				// surfaced simply as Octad, so today there is no header, just cards.
				let html = '<div class="rating-summary">';
				let curMode = null;
				rows.forEach((r) => {
					const mode = r.mode || '';
					if (mode !== curMode) {
						if (curMode !== null) { html += '</div>'; } // close prior grid
						if (mode) { html += `<div class="rating-mode">${esc(mode)}</div>`; }
						html += '<div class="rating-grid">';
						curMode = mode;
					}
					html += '<div class="rating-card">'
						+ `<span class="rating-tc">${esc(r.timeControl)}</span>`
						+ `<span class="rating-val">${esc(r.rating)}</span>`
						+ `<span class="rating-speed">${esc(r.speed)}</span>`
						+ '</div>';
				});
				html += '</div></div>'; // close last grid + summary
				ratingsSummary.innerHTML = html;
			} catch (e) {
				ratingsSummary.dataset.loaded = 'false'; // allow a retry on reopen
			}
		});
	}

	// change password: confirm client-side, POST, then show success (the server
	// signs out the account's other sessions; this one stays)
	const pwForm = document.getElementById('passwordForm');
	if (pwForm) {
		const okEl = pwForm.querySelector('[data-auth-ok]');
		pwForm.addEventListener('submit', async (e) => {
			e.preventDefault();
			showError(pwForm);
			if (okEl) { okEl.classList.add('hidden'); }
			if (pwForm.new.value !== pwForm.confirm.value) {
				showError(pwForm, 'New passwords do not match.');
				return;
			}
			try {
				const { status, data } = await post('/api/auth/password', {
					current: pwForm.current.value,
					new: pwForm.new.value,
				});
				if (status === 204) {
					pwForm.reset();
					if (okEl) { okEl.classList.remove('hidden'); }
					return;
				}
				showError(pwForm, data.error || 'Could not change password.');
			} catch (err) {
				showError(pwForm, 'Network error — try again.');
			}
		});
	}

	// active sessions: lazy-load the fragment the first time the section opens,
	// and reload it after a revoke (event-delegated on the list body)
	const sessionsDetails = document.getElementById('sessionsDetails');
	const sessionsBody = document.getElementById('sessionsBody');
	if (sessionsDetails && sessionsBody) {
		const loadSessions = async () => {
			try {
				const res = await fetch('/api/auth/sessions');
				sessionsBody.innerHTML = res.ok
					? await res.text()
					: '<p class="auth-hint">Could not load sessions.</p>';
				sessionsBody.dataset.loaded = 'true';
			} catch (err) {
				sessionsBody.innerHTML = '<p class="auth-hint">Could not load sessions.</p>';
			}
		};
		sessionsDetails.addEventListener('toggle', () => {
			if (sessionsDetails.open && sessionsBody.dataset.loaded !== 'true') {
				loadSessions();
			}
		});
		sessionsBody.addEventListener('click', async (e) => {
			const btn = e.target.closest('[data-session-id]');
			if (!btn) { return; }
			btn.disabled = true;
			try {
				await post('/api/auth/sessions/revoke', { id: Number(btn.dataset.sessionId) });
			} catch (err) { /* reload reflects the result either way */ }
			await loadSessions();
		});
	}

	// --- two-factor & passkey management modal -----------------------------
	initSecurityModal();

	// --- edit profile modal (email + one-time casing-only username change) --
	initEditProfileModal();

	// --- logged-out: auth modal --------------------------------------------
	const modal = document.getElementById('modalAccount');
	if (!modal) { return; }

	const authTabs = document.getElementById('authTabs');
	const tabs = modal.querySelectorAll('[data-auth-tab]');
	const mfaStep = document.getElementById('mfaStep');
	const forms = {
		login: modal.querySelector('[data-auth-form="login"]'),
		register: modal.querySelector('[data-auth-form="register"]'),
	};

	// restore the modal to its default (login tab, no MFA step) — called when
	// the login button opens it and when it is dismissed, so a half-finished
	// second factor never lingers on reopen.
	const resetAuthModal = () => {
		if (mfaStep) { mfaStep.classList.remove('flex'); mfaStep.classList.add('hidden'); }
		if (authTabs) { authTabs.classList.remove('hidden'); }
		activateTab('login');
	};

	const activateTab = (which) => {
		tabs.forEach((t) => t.classList.toggle('is-active', t.dataset.authTab === which));
		Object.keys(forms).forEach((k) => {
			const form = forms[k];
			if (!form) { return; }
			form.classList.toggle('hidden', k !== which);
			form.classList.toggle('flex', k === which);
		});
	};

	// tab switching: swap the active pill and the visible form.
	tabs.forEach((tab) => {
		tab.addEventListener('click', () => {
			activateTab(tab.dataset.authTab);
			const first = forms[tab.dataset.authTab] && forms[tab.dataset.authTab].querySelector('input');
			if (first) { first.focus(); }
		});
	});

	// reset MFA state whenever the modal is opened or dismissed
	const loginBtn = document.getElementById('loginButton');
	if (loginBtn) { loginBtn.addEventListener('click', resetAuthModal); }
	const modalCloseBtn = modal.querySelector('.modal-close');
	if (modalCloseBtn) { modalCloseBtn.addEventListener('click', resetAuthModal); }
	modal.addEventListener('click', (e) => { if (e.target === modal) { resetAuthModal(); } });

	// --- login-time second factor ------------------------------------------
	let pendingToken = '';

	// enter the MFA step: hide the tabs + login/register forms, reveal the
	// second-factor UI configured for the offered methods.
	const enterMFA = (data) => {
		pendingToken = data.pending || '';
		const methods = data.methods || {};
		if (authTabs) { authTabs.classList.add('hidden'); }
		Object.values(forms).forEach((f) => { if (f) { f.classList.add('hidden'); f.classList.remove('flex'); } });
		if (mfaStep) { mfaStep.classList.remove('hidden'); mfaStep.classList.add('flex'); }
		mfaMethods = methods;
		// default to TOTP, else passkey, else recovery
		setMFAMode(methods.totp ? 'totp' : (methods.passkey && webAuthnSupported() ? 'passkey' : 'recovery'));
	};

	let mfaMethods = {};
	const mfaPrompt = mfaStep && mfaStep.querySelector('[data-mfa-prompt]');
	const mfaLabel = mfaStep && mfaStep.querySelector('[data-mfa-label]');
	const mfaCodeForm = document.getElementById('mfaCodeForm');
	const mfaPasskeyBtn = document.getElementById('mfaPasskeyBtn');
	const mfaAlts = mfaStep ? mfaStep.querySelectorAll('[data-mfa-alt]') : [];

	// configure the step for one method: prompt/label text, which control is
	// shown (code form vs passkey button), and which alternate methods to offer.
	const setMFAMode = (mode) => {
		showError(mfaStep);
		const passkeyOK = mfaMethods.passkey && webAuthnSupported();
		if (mode === 'passkey' && mfaCodeForm) { mfaCodeForm.classList.add('hidden'); }
		else if (mfaCodeForm) { mfaCodeForm.classList.remove('hidden'); }
		if (mfaPasskeyBtn) { mfaPasskeyBtn.classList.toggle('hidden', mode !== 'passkey'); }

		if (mode === 'totp' && mfaPrompt) {
			mfaPrompt.textContent = 'Enter the 6-digit code from your authenticator app.';
			if (mfaLabel) { mfaLabel.textContent = 'Authentication code'; }
		} else if (mode === 'recovery' && mfaPrompt) {
			mfaPrompt.textContent = 'Enter one of your recovery codes.';
			if (mfaLabel) { mfaLabel.textContent = 'Recovery code'; }
		} else if (mode === 'passkey' && mfaPrompt) {
			mfaPrompt.textContent = 'Verify with a passkey on this device.';
		}
		mfaStep.dataset.mode = mode;
		if (mfaCodeForm) { mfaCodeForm.reset(); }

		// alt links: show every available method except the current one
		mfaAlts.forEach((b) => {
			const m = b.dataset.mfaAlt;
			let avail = false;
			if (m === 'totp') { avail = !!mfaMethods.totp; }
			else if (m === 'recovery') { avail = !!mfaMethods.recovery; }
			else if (m === 'passkey') { avail = passkeyOK; }
			b.classList.toggle('hidden', !avail || m === mode);
		});

		const input = mfaCodeForm && mfaCodeForm.querySelector('input[name="code"]');
		if (mode !== 'passkey' && input) { input.focus(); }
	};

	mfaAlts.forEach((b) => b.addEventListener('click', () => setMFAMode(b.dataset.mfaAlt)));

	if (mfaCodeForm) {
		mfaCodeForm.addEventListener('submit', async (e) => {
			e.preventDefault();
			showError(mfaStep);
			const mode = mfaStep.dataset.mode;
			const path = mode === 'recovery' ? '/api/auth/login/recovery' : '/api/auth/login/totp';
			try {
				const { status, data } = await post(path, {
					pending: pendingToken,
					code: mfaCodeForm.code.value.trim(),
				});
				if (status === 200) { window.location.reload(); return; }
				showError(mfaStep, data.error || 'That did not work — try again.');
			} catch (err) {
				showError(mfaStep, 'Network error — try again.');
			}
		});
	}

	if (mfaPasskeyBtn) {
		mfaPasskeyBtn.addEventListener('click', async () => {
			showError(mfaStep);
			mfaPasskeyBtn.disabled = true;
			try {
				const begin = await post('/api/auth/login/webauthn/begin?pending=' + encodeURIComponent(pendingToken), null);
				if (begin.status !== 200 || !begin.data.publicKey) {
					showError(mfaStep, begin.data.error || 'Could not start passkey verification.');
					return;
				}
				const assertion = await navigator.credentials.get({ publicKey: prepAssertion(begin.data.publicKey) });
				const fin = await post('/api/auth/login/webauthn/finish?pending=' + encodeURIComponent(pendingToken), credentialToJSON(assertion));
				if (fin.status === 200) { window.location.reload(); return; }
				showError(mfaStep, fin.data.error || 'Passkey verification failed.');
			} catch (err) {
				showError(mfaStep, 'Passkey verification was cancelled.');
			} finally {
				mfaPasskeyBtn.disabled = false;
			}
		});
	}

	// login submit
	if (forms.login) {
		forms.login.addEventListener('submit', async (e) => {
			e.preventDefault();
			const form = forms.login;
			showError(form);
			const body = {
				username: form.username.value.trim(),
				password: form.password.value,
			};
			try {
				const { status, data } = await post('/api/auth/login', body);
				if (status === 200 && data.mfa) { enterMFA(data); return; }
				if (status === 200) { window.location.reload(); return; }
				showError(form, data.error || 'Login failed — try again.');
			} catch (err) {
				showError(form, 'Network error — try again.');
			}
		});
	}

	// register submit + live username availability probe (debounced)
	if (forms.register) {
		const form = forms.register;
		const availEl = form.querySelector('[data-auth-avail]');
		let availTimer = null;

		form.username.addEventListener('input', () => {
			if (!availEl) { return; }
			availEl.textContent = '';
			availEl.classList.remove('text-win', 'text-loss');
			clearTimeout(availTimer);
			const u = form.username.value.trim();
			if (u.length < 3) { return; }
			availTimer = setTimeout(async () => {
				try {
					const res = await fetch(
						'/api/auth/username-available?u=' + encodeURIComponent(u));
					const data = await res.json();
					if (form.username.value.trim() !== u) { return; } // stale
					if (data.available) {
						availEl.textContent = u + ' is available';
						availEl.classList.add('text-win');
					} else {
						availEl.textContent = data.reason || 'unavailable';
						availEl.classList.add('text-loss');
					}
				} catch (err) { /* probe is best-effort */ }
			}, 400);
		});

		form.addEventListener('submit', async (e) => {
			e.preventDefault();
			showError(form);
			const body = {
				username: form.username.value.trim(),
				password: form.password.value,
				email: form.email.value.trim(),
			};
			try {
				const { status, data } = await post('/api/auth/register', body);
				if (status === 200) {
					window.location.reload();
					return;
				}
				showError(form, data.error || 'Registration failed — try again.');
			} catch (err) {
				showError(form, 'Network error — try again.');
			}
		});
	}

	// --- room anonymous "create account" shim ------------------------------
	// The thin room-page banner (view/room.templ roomAnonCta). Create-account
	// opens this modal straight to the register tab; × dismisses the shim and
	// persists it so the head no-flash script keeps it hidden on future loads.
	const ctaCreate = document.getElementById('roomCtaCreate');
	if (ctaCreate) {
		ctaCreate.addEventListener('click', () => {
			resetAuthModal();
			activateTab('register');
			modal.classList.add('open');
			const first = forms.register && forms.register.querySelector('input');
			if (first) { first.focus(); }
		});
	}
	const ctaDismiss = document.getElementById('roomCtaDismiss');
	if (ctaDismiss) {
		ctaDismiss.addEventListener('click', () => {
			try { localStorage.setItem('roomCtaDismissed', '1'); } catch (e) { /* private mode */ }
			document.documentElement.dataset.roomcta = 'off';
		});
	}

	// ========================================================================
	// Edit profile modal (logged in): the optional account email and the one
	// allowed casing-only username change. The username is prefilled by the
	// server; on open we fetch GET /api/auth/profile to fill the email and lock
	// the username controls once the single change has been used.
	// ========================================================================
	function initEditProfileModal() {
		const epModal = document.getElementById('modalEditProfile');
		const openBtn = document.getElementById('editProfileButton');
		if (!epModal || !openBtn) { return; }

		const usernameForm = document.getElementById('usernameForm');
		const emailForm = document.getElementById('emailForm');

		// reset a form's transient error/success lines back to hidden
		const resetForm = (form) => {
			if (!form) { return; }
			showError(form);
			const ok = form.querySelector('[data-auth-ok]');
			if (ok) { ok.classList.add('hidden'); }
		};

		// pull current email + username-change availability into the modal
		const loadProfile = async () => {
			try {
				const res = await fetch('/api/auth/profile');
				if (!res.ok) { throw new Error('profile'); }
				const p = await res.json();
				if (emailForm) { emailForm.email.value = p.email || ''; }
				if (usernameForm) {
					if (p.username) { usernameForm.username.value = p.username; }
					const hint = usernameForm.querySelector('[data-username-hint]');
					const submit = usernameForm.querySelector('[data-username-submit]');
					const avail = !!p.usernameChangeAvailable;
					usernameForm.username.disabled = !avail;
					if (submit) { submit.disabled = !avail; }
					if (hint) {
						hint.textContent = avail
							? 'You can change your username once, and only to change its capitalization.'
							: 'You have already used your one username change.';
					}
				}
			} catch (e) { /* leave the server-prefilled username; email stays blank */ }
		};

		const open = () => {
			resetForm(usernameForm);
			resetForm(emailForm);
			epModal.classList.add('open');
			loadProfile();
		};
		const close = () => { epModal.classList.remove('open'); };

		openBtn.addEventListener('click', () => {
			// dismiss the profile popover (and its scrim) before opening
			const pp = document.getElementById('profilePopover');
			if (pp) { pp.classList.add('hidden'); }
			const scrim = document.getElementById('menuScrim');
			if (scrim) { scrim.classList.remove('is-open'); }
			open();
		});
		const closeBtn = epModal.querySelector('.modal-close');
		if (closeBtn) { closeBtn.addEventListener('click', close); }
		epModal.addEventListener('click', (e) => { if (e.target === epModal) { close(); } });

		// username: casing-only, once. On success reload so the header (and any
		// live-game clocks) re-render the new capitalization.
		if (usernameForm) {
			const okEl = usernameForm.querySelector('[data-auth-ok]');
			usernameForm.addEventListener('submit', async (e) => {
				e.preventDefault();
				resetForm(usernameForm);
				const submit = usernameForm.querySelector('[data-username-submit]');
				if (submit) { submit.disabled = true; }
				try {
					const { status, data } = await post('/api/auth/username', {
						username: usernameForm.username.value.trim(),
					});
					if (status === 200) {
						if (okEl) { okEl.classList.remove('hidden'); }
						setTimeout(() => window.location.reload(), 700);
						return;
					}
					showError(usernameForm, data.error || 'Could not change username.');
					if (submit) { submit.disabled = false; }
				} catch (err) {
					showError(usernameForm, 'Network error — try again.');
					if (submit) { submit.disabled = false; }
				}
			});
		}

		// email: set / change / clear
		if (emailForm) {
			const okEl = emailForm.querySelector('[data-auth-ok]');
			emailForm.addEventListener('submit', async (e) => {
				e.preventDefault();
				resetForm(emailForm);
				try {
					const { status, data } = await post('/api/auth/email', {
						email: emailForm.email.value.trim(),
					});
					if (status === 200) {
						if (okEl) { okEl.classList.remove('hidden'); }
						return;
					}
					showError(emailForm, data.error || 'Could not save email.');
				} catch (err) {
					showError(emailForm, 'Network error — try again.');
				}
			});
		}
	}

	// ========================================================================
	// Two-factor & passkey management modal (logged in). The body is rendered
	// entirely here from GET /api/auth/mfa/status and swapped through the enroll
	// ceremonies. Recovery codes are shown once, at generation.
	// ========================================================================
	function initSecurityModal() {
		const secModal = document.getElementById('modalSecurity');
		const body = document.getElementById('securityModalBody');
		const openBtn = document.getElementById('securityButton');
		if (!secModal || !body || !openBtn) { return; }

		const open = () => { secModal.classList.add('open'); loadStatus(); };
		const close = () => { secModal.classList.remove('open'); };

		openBtn.addEventListener('click', () => {
			// dismiss the profile popover (and its scrim) before opening
			const pp = document.getElementById('profilePopover');
			if (pp) { pp.classList.add('hidden'); }
			const scrim = document.getElementById('menuScrim');
			if (scrim) { scrim.classList.remove('is-open'); }
			open();
		});
		const closeBtn = secModal.querySelector('.modal-close');
		if (closeBtn) { closeBtn.addEventListener('click', close); }
		secModal.addEventListener('click', (e) => { if (e.target === secModal) { close(); } });

		const setBusy = (btn, busy) => { if (btn) { btn.disabled = busy; } };

		// --- status view ---
		const loadStatus = async () => {
			body.innerHTML = '<p class="auth-hint">Loading…</p>';
			try {
				const res = await fetch('/api/auth/mfa/status');
				if (!res.ok) { throw new Error('status'); }
				renderStatus(await res.json());
			} catch (err) {
				body.innerHTML = '<p class="auth-hint">Could not load security settings.</p>';
			}
		};

		const renderStatus = (s) => {
			const totpOn = !!s.totp;
			const passkeys = s.passkeys || [];
			const anyMFA = totpOn || passkeys.length > 0;
			const pkSupported = webAuthnSupported();

			let pkRows = passkeys.map((p) => `
				<div class="pk-row">
					<div style="min-width:0">
						<div class="pk-name">${esc(p.nickname)}</div>
						<div class="pk-meta">Added ${esc(p.addedAt)}${p.lastUsed ? ' · used ' + esc(p.lastUsed) : ''}</div>
					</div>
					<button type="button" class="pk-del" data-pk-del="${p.id}">Remove</button>
				</div>`).join('');
			if (!pkRows) { pkRows = '<p class="auth-hint" style="margin-top:.4rem">No passkeys yet.</p>'; }

			body.innerHTML = `
				<div class="mfa-section">
					<div class="mfa-head">
						<div>
							<div class="mfa-title">Authenticator app</div>
							<div class="mfa-desc">Time-based codes from an app like Google Authenticator or 1Password.</div>
						</div>
						<span class="${totpOn ? 'mfa-on' : 'mfa-off'}">${totpOn ? 'On' : 'Off'}</span>
					</div>
					<button type="button" class="btn btn-ghost w-full justify-center py-1.5 text-sm mt-2.5" data-act="${totpOn ? 'totp-disable' : 'totp-enable'}">
						${totpOn ? 'Turn off' : 'Set up'}
					</button>
				</div>

				<div class="mfa-section">
					<div class="mfa-head">
						<div>
							<div class="mfa-title">Passkeys</div>
							<div class="mfa-desc">Sign in with a fingerprint, face, or security key.</div>
						</div>
					</div>
					<div class="pk-list">${pkRows}</div>
					<button type="button" class="btn btn-ghost w-full justify-center py-1.5 text-sm mt-2.5" data-act="passkey-add" ${pkSupported ? '' : 'disabled title="This browser does not support passkeys"'}>
						Add a passkey
					</button>
				</div>

				${anyMFA ? `
				<div class="mfa-section">
					<div class="mfa-head">
						<div>
							<div class="mfa-title">Recovery codes</div>
							<div class="mfa-desc">${s.recoveryRemaining} unused code${s.recoveryRemaining === 1 ? '' : 's'} remaining.</div>
						</div>
					</div>
					<button type="button" class="btn btn-ghost w-full justify-center py-1.5 text-sm mt-2.5" data-act="recovery-regen">Generate new codes</button>
				</div>` : ''}
			`;

			body.querySelectorAll('[data-act]').forEach((btn) => {
				btn.addEventListener('click', () => handleAct(btn.dataset.act));
			});
			body.querySelectorAll('[data-pk-del]').forEach((btn) => {
				btn.addEventListener('click', () => deletePasskey(btn.dataset.pkDel));
			});
		};

		const handleAct = (act) => {
			if (act === 'totp-enable') { totpEnableGate(); }
			else if (act === 'totp-disable') { totpDisableGate(); }
			else if (act === 'passkey-add') { passkeyAddGate(); }
			else if (act === 'recovery-regen') { recoveryRegenGate(); }
		};

		// --- a generic password-gate view ---
		// renders a titled form asking for the current password (+ optional
		// extra fields), calls onSubmit(values, scope) on submit, and offers a
		// Back link to the status view.
		const passwordGate = (opts) => {
			body.innerHTML = `
				<h3 class="text-md font-semibold text-fg">${esc(opts.title)}</h3>
				${opts.desc ? `<p class="auth-hint mt-1">${esc(opts.desc)}</p>` : ''}
				<form class="mt-3 flex flex-col gap-3" novalidate>
					${opts.extra || ''}
					<label class="auth-label">
						Current password
						<input class="auth-input" name="password" type="password" autocomplete="current-password" required/>
					</label>
					<p class="auth-error hidden" data-auth-error role="alert"></p>
					<div class="flex gap-2">
						<button type="button" class="btn btn-ghost flex-1 justify-center py-1.5 text-sm" data-back>Back</button>
						<button type="submit" class="btn btn-primary flex-1 justify-center py-1.5 text-sm">${esc(opts.submit)}</button>
					</div>
				</form>`;
			const form = body.querySelector('form');
			form.querySelector('[data-back]').addEventListener('click', loadStatus);
			form.addEventListener('submit', async (e) => {
				e.preventDefault();
				showError(form);
				const submitBtn = form.querySelector('button[type="submit"]');
				setBusy(submitBtn, true);
				try {
					await opts.onSubmit(form);
				} catch (err) {
					showError(form, 'Something went wrong — try again.');
				} finally {
					setBusy(submitBtn, false);
				}
			});
			const first = form.querySelector('input');
			if (first) { first.focus(); }
		};

		// --- TOTP enable: password -> QR/secret -> confirm code -> codes ---
		const totpEnableGate = () => passwordGate({
			title: 'Set up authenticator app',
			desc: 'Confirm your password to begin.',
			submit: 'Continue',
			onSubmit: async (form) => {
				const { status, data } = await post('/api/auth/totp/begin', { password: form.password.value });
				if (status !== 200) { showError(form, data.error || 'Could not start setup.'); return; }
				totpEnrollView(data);
			},
		});

		const totpEnrollView = (enroll) => {
			body.innerHTML = `
				<h3 class="text-md font-semibold text-fg">Scan this code</h3>
				<p class="auth-hint mt-1">Scan with your authenticator app, or enter the key manually, then type the 6-digit code to confirm.</p>
				<div class="qr-frame"><img alt="TOTP QR code" src="${esc(enroll.qr)}"/></div>
				<div class="totp-secret">${esc(enroll.secret)}</div>
				<form class="mt-3 flex flex-col gap-3" novalidate>
					<label class="auth-label">
						6-digit code
						<input class="auth-input" name="code" inputmode="numeric" autocomplete="one-time-code" autocapitalize="off" spellcheck="false" required/>
					</label>
					<p class="auth-error hidden" data-auth-error role="alert"></p>
					<div class="flex gap-2">
						<button type="button" class="btn btn-ghost flex-1 justify-center py-1.5 text-sm" data-back>Cancel</button>
						<button type="submit" class="btn btn-primary flex-1 justify-center py-1.5 text-sm">Confirm</button>
					</div>
				</form>`;
			const form = body.querySelector('form');
			form.querySelector('[data-back]').addEventListener('click', loadStatus);
			form.addEventListener('submit', async (e) => {
				e.preventDefault();
				showError(form);
				const submitBtn = form.querySelector('button[type="submit"]');
				setBusy(submitBtn, true);
				try {
					const { status, data } = await post('/api/auth/totp/confirm', { code: form.code.value.trim() });
					if (status !== 200) { showError(form, data.error || 'That code did not match.'); return; }
					if (data.recoveryCodes && data.recoveryCodes.length) {
						recoveryCodesView(data.recoveryCodes, 'Two-factor is on — save your recovery codes');
					} else {
						loadStatus();
					}
				} finally {
					setBusy(submitBtn, false);
				}
			});
			const input = form.querySelector('input[name="code"]');
			if (input) { input.focus(); }
		};

		// --- TOTP disable ---
		const totpDisableGate = () => passwordGate({
			title: 'Turn off authenticator app',
			desc: 'Confirm your password to disable two-factor codes.',
			submit: 'Turn off',
			onSubmit: async (form) => {
				const { status, data } = await post('/api/auth/totp/disable', { password: form.password.value });
				if (status === 204) { loadStatus(); return; }
				showError(form, data.error || 'Could not disable.');
			},
		});

		// --- passkey add: password + nickname -> create -> finish ---
		const passkeyAddGate = () => {
			if (!webAuthnSupported()) { return; }
			passwordGate({
				title: 'Add a passkey',
				desc: 'Confirm your password, then follow your device prompt.',
				submit: 'Add passkey',
				extra: `<label class="auth-label">Name (optional)<input class="auth-input" name="nickname" type="text" maxlength="40" placeholder="e.g. My laptop"/></label>`,
				onSubmit: async (form) => {
					const nickname = form.nickname ? form.nickname.value.trim() : '';
					const begin = await post('/api/auth/webauthn/register/begin', { password: form.password.value });
					if (begin.status !== 200 || !begin.data.publicKey) {
						showError(form, begin.data.error || 'Could not start passkey setup.');
						return;
					}
					let cred;
					try {
						cred = await navigator.credentials.create({ publicKey: prepCreation(begin.data.publicKey) });
					} catch (err) {
						showError(form, 'Passkey setup was cancelled.');
						return;
					}
					const fin = await post('/api/auth/webauthn/register/finish?nickname=' + encodeURIComponent(nickname), credentialToJSON(cred));
					if (fin.status !== 200) { showError(form, fin.data.error || 'Could not save the passkey.'); return; }
					if (fin.data.recoveryCodes && fin.data.recoveryCodes.length) {
						recoveryCodesView(fin.data.recoveryCodes, 'Passkey added — save your recovery codes');
					} else {
						loadStatus();
					}
				},
			});
		};

		const deletePasskey = async (id) => {
			try {
				await post('/api/auth/webauthn/credentials/delete', { id: Number(id) });
			} catch (err) { /* reload reflects the result */ }
			loadStatus();
		};

		// --- recovery codes: regenerate + one-time display ---
		const recoveryRegenGate = () => passwordGate({
			title: 'Generate new recovery codes',
			desc: 'Your old codes stop working. Confirm your password to continue.',
			submit: 'Generate',
			onSubmit: async (form) => {
				const { status, data } = await post('/api/auth/recovery/regenerate', { password: form.password.value });
				if (status !== 200) { showError(form, data.error || 'Could not generate codes.'); return; }
				recoveryCodesView(data.recoveryCodes || [], 'Save your recovery codes');
			},
		});

		const recoveryCodesView = (codes, title) => {
			const grid = codes.map((c) => `<div class="recovery-code">${esc(c)}</div>`).join('');
			body.innerHTML = `
				<h3 class="text-md font-semibold text-fg">${esc(title)}</h3>
				<p class="recovery-warn mt-1">Store these somewhere safe. Each works once, and this is the only time they are shown.</p>
				<div class="recovery-grid">${grid}</div>
				<div class="flex gap-2">
					<button type="button" class="btn btn-ghost flex-1 justify-center py-1.5 text-sm" data-copy>Copy all</button>
					<button type="button" class="btn btn-primary flex-1 justify-center py-1.5 text-sm" data-done>Done</button>
				</div>`;
			body.querySelector('[data-done]').addEventListener('click', loadStatus);
			const copyBtn = body.querySelector('[data-copy]');
			copyBtn.addEventListener('click', async () => {
				try {
					await navigator.clipboard.writeText(codes.join('\n'));
					copyBtn.textContent = 'Copied';
				} catch (err) { copyBtn.textContent = 'Copy failed'; }
			});
		};
	}
})();

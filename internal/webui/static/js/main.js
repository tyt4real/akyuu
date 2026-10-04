// Minimal JavaScript for akyuu web UI
// Toggle post form
document.addEventListener('DOMContentLoaded', function() {
    const btn = document.getElementById('toggleFormBtn');
    const form = document.getElementById('postFormContainer');

    if (btn && form) {
        btn.addEventListener('click', () => {
            form.classList.toggle('hidden');
            if (!form.classList.contains('hidden')) {
                form.scrollIntoView({ behavior: 'smooth' });
            }
        });
    }

    // Captcha reload
    const captchaImg = document.getElementById('captcha-img');
    const captchaReload = document.querySelector('a[href^="/captcha?"]');
    if (captchaReload && captchaImg) {
        captchaReload.addEventListener('click', function(e) {
            e.preventDefault();
            captchaImg.src = '/captcha?' + Date.now();
            return false;
        });
    }

    // Thread updater (for watched threads)
    if (typeof threadUpdater !== 'undefined' && threadUpdater) {
        // Thread updater logic would go here
    }
});

// Expand image function
function expandImage(element, url) {
    window.open(url, '_blank');
    return false;
}

// Toggle image spoiler
function toggleImageSpoiler(element) {
    const spoiler = element.closest('.image-spoiler');
    if (spoiler) {
        spoiler.classList.toggle('revealed');
    }
    return false;
}

// Thread watcher functions
function watchThread(threadId, boardUri) {
    // Implementation for watching threads
}

function hideThread(threadId) {
    // Implementation for hiding threads
}

// Settings modal
function openSettings() {
    const modal = document.getElementById('settings-modal');
    if (modal) modal.classList.remove('hidden');
}

function closeSettings() {
    const modal = document.getElementById('settings-modal');
    if (modal) modal.classList.add('hidden');
}

// Settings save
function saveSettings() {
    // Save settings to localStorage and send to server
    closeSettings();
}

// Initialize on load
document.addEventListener('DOMContentLoaded', function() {
    // Add smooth scroll for anchor links
    document.querySelectorAll('a[href^="#"]').forEach(anchor => {
        anchor.addEventListener('click', function(e) {
            const target = document.querySelector(this.getAttribute('href'));
            if (target) {
                e.preventDefault();
                target.scrollIntoView({ behavior: 'smooth' });
            }
        });
    });

    // Close modals on escape key
    document.addEventListener('keydown', function(e) {
        if (e.key === 'Escape') {
            closeSettings();
            // Close other modals
            document.querySelectorAll('[id$="-modal"]').forEach(modal => {
                modal.classList.add('hidden');
            });
        }
    });
});
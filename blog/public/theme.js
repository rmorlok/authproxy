const storageKey = 'authproxy-blog-theme';
const documentElement = document.documentElement;
const toggle = document.querySelector('[data-theme-toggle]');
const savedTheme = localStorage.getItem(storageKey);

if (savedTheme === 'light' || savedTheme === 'dark') {
  documentElement.dataset.theme = savedTheme;
}

toggle?.addEventListener('click', () => {
  const currentTheme = documentElement.dataset.theme;
  const systemIsDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
  const nextTheme = currentTheme === 'dark' || (!currentTheme && systemIsDark) ? 'light' : 'dark';

  documentElement.dataset.theme = nextTheme;
  localStorage.setItem(storageKey, nextTheme);
});

// PostCSS config — Docsy's SCSS pipeline expects this in the site root
// rather than reading it from the Hugo module cache.
module.exports = {
  plugins: [
    require('autoprefixer')({ overrideBrowserslist: ['last 2 versions'] }),
  ],
};

/** @type {import('next').NextConfig} */
const nextConfig = {
  
};

module.exports = {
  ...nextConfig,
  async rewrites() {
    return [
      {
        source: '/api/:slug*',
        destination: 'http://127.0.0.1:4444/api/:slug*'
      },
    ]
  },
};

/* eslint-disable @typescript-eslint/no-var-requires */
const {
  createVanillaExtractPlugin
} = require('@vanilla-extract/next-plugin');
const withVanillaExtract = createVanillaExtractPlugin();

/** @type {import('next').NextConfig} */
const nextConfig = {
  
};

module.exports = {
  ...withVanillaExtract(nextConfig),
  async rewrites() {
    return [
      {
        source: '/api/:slug*',
        destination: 'http://127.0.0.1:4444/api/:slug*'
      },
    ]
  },
};

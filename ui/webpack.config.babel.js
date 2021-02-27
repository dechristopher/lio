import merge from 'webpack-merge';

const baseConfig = require('./webpack/base');
const devConfig = require('./webpack/dev');
const prodConfig = require('./webpack/prod');
const {isProd} = require('./webpack/utils/env');

export default () =>
    isProd ? merge(baseConfig, prodConfig) : merge(baseConfig, devConfig);
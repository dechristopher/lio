import merge from 'webpack-merge';

const baseConfig = require('./webpack/base');
const devConfig = require('./webpack/dev');
const prodConfig = require('./webpack/prod');
const {isProd} = require('./webpack/utils/env');

module.exports = () => {
    const parsedConfig = isProd
        ? merge(baseConfig, prodConfig)
        : merge(baseConfig, devConfig)

    // Object.defineProperty(RegExp.prototype, 'toJSON', {
    //     value: RegExp.prototype.toString,
    // });
    //
    // fs.writeFileSync("./webpack.config.parsed.json", JSON.stringify(parsedConfig.default))

    return parsedConfig.default;
};

/**
 * Created by: Andrey Polyakov (andrey@polyakov.im)
 */
import path from 'path';
import {TsconfigPathsPlugin} from "tsconfig-paths-webpack-plugin"

import {devServerUrl} from './config';
import entry from './entry';
import optimization from './optimization';
import * as plugins from './plugins';
import * as rules from './rules';
import {isDevServer, isProd} from './utils/env';
import {arrayFilterEmpty} from './utils/helpers';

export default {
    context: __dirname,
    target: isDevServer ? 'web' : ['web', 'es5'],
    mode: isProd ? 'production' : 'development',
    entry: {
        main: [
            ...(isDevServer
                ? [
                    "react-hot-loader/patch", // activate HMR for React
                    "webpack-dev-server/client?http://localhost:8080", // bundle the client for webpack-dev-server and connect to the provided endpoint
                    "webpack/hot/only-dev-server", // bundle the client for hot reloading, only- means to only hot reload for successful updates
                ]
                : []),
            ...entry.main
        ]
    },
    output: {
        path: path.join(__dirname, '../dist'),
        publicPath: '/',
        filename: isDevServer
            ? '[name].[fullhash].js'
            : '[name].[contenthash].js',
    },
    module: {
        rules: arrayFilterEmpty([
            rules.javascriptRule,
            rules.typescriptRule,
            rules.htmlRule,
            rules.imagesRule,
            rules.fontsRule,
            rules.cssRule,
            rules.hmrRule,
            ...rules.lessRules,
            ...rules.sassRules,
            ...rules.svgRules,
        ]),
    },
    plugins: arrayFilterEmpty([
        plugins.htmlWebpackPlugin,
        plugins.providePlugin,
        plugins.definePlugin,
        plugins.forkTsCheckerWebpackPlugin,
        plugins.esLintPlugin,
        plugins.copyPlugin,
    ]),
    resolve: {
        plugins: [new TsconfigPathsPlugin()],
        extensions: ['.tsx', '.ts', '.js', '.jsx'],
    },
    optimization,
};

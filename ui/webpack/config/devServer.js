import isWindows from 'is-windows';
import {resolve} from "path";

import {rootDir} from "../utils/env";

const defaultPort = 8080;

const devServerHost = isWindows() ? '127.0.0.1' : 'localhost';

export const devServerUrl = `http://${devServerHost}:${defaultPort}`;

export const devServerConfig = {
    // open: true,
    contentBase: resolve( rootDir, "./src" ),
    contentBasePublicPath: "/",
    // do not print bundle build stats
    // noInfo: true,
    // enable HMR
    hot: true,
    // embed the webpack-dev-server runtime into the bundle
    inline: true,
    // serve index.html in place of 404 responses to allow HTML5 history
    historyApiFallback: true,
    // port: 80,
    host: devServerHost,
    disableHostCheck: true, // insecure
    transportMode: "ws",
    compress: true,

    // clientLogLevel: 'warning',
    // open: true,
    stats: 'errors-only'
};
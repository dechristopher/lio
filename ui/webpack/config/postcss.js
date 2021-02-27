const tw = require("tailwindcss");
import {join} from "path";

import {isProd, rootDir} from '../utils/env';
import {arrayFilterEmpty} from '../utils/helpers';

module.exports = {
    plugins: [tw("./tailwind.config.js"), require("autoprefixer")],
};

module.exports = () => {
    const plugins = arrayFilterEmpty([
        tw(join(rootDir, "./tailwind.config.js")),
        'autoprefixer',
        isProd ? 'cssnano' : null,
    ]);
    return {
        plugins,
    };
};
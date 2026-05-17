import fs from 'fs';
import { execSync } from 'child_process';

const outputDir = 'src/gen';

// Recursively create parent directories if they don't exist
if (!fs.existsSync(outputDir)) {
    fs.mkdirSync(outputDir, { recursive: true });
}

// Run the pbjs command for actual code generation
execSync(`pbjs -t static-module -w esm -o ${outputDir}/types.js --dts ../api/message.proto`);

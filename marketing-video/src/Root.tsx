import React from 'react';
import {Composition} from 'remotion';
import {MirageVideo, FPS, DURATION_IN_FRAMES, WIDTH, HEIGHT} from './MirageVideo';

export const RemotionRoot: React.FC = () => {
  return (
    <>
      <Composition
        id="MirageVideo"
        component={MirageVideo}
        durationInFrames={DURATION_IN_FRAMES}
        fps={FPS}
        width={WIDTH}
        height={HEIGHT}
      />
    </>
  );
};

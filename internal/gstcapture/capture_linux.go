//go:build linux && vim4

// Package gstcapture delivers complete GStreamer buffers with their media timing.
package gstcapture

/*
#cgo pkg-config: gstreamer-app-1.0 gstreamer-video-1.0
#include <gst/gst.h>
#include <gst/app/gstappsink.h>
#include <gst/video/video.h>
#include <stdlib.h>

static GstElement *open_pipeline(const char *description, char **error) {
    GError *e = NULL;
    gst_init(NULL, NULL);
    GstElement *pipeline = gst_parse_launch(description, &e);
    if (e) {
        *error = g_strdup(e->message);
        g_error_free(e);
        if (pipeline) gst_object_unref(pipeline);
        return NULL;
    }
    return pipeline;
}
static int sample_data(GstSample *sample, GstMapInfo *map,
                      guint64 *pts, guint64 *duration, int *width, int *height, int *keyframe) {
    GstBuffer *buffer = gst_sample_get_buffer(sample);
    if (!buffer || !gst_buffer_map(buffer, map, GST_MAP_READ)) return 0;
    *pts = GST_BUFFER_PTS(buffer);
    *keyframe = !GST_BUFFER_FLAG_IS_SET(buffer, GST_BUFFER_FLAG_DELTA_UNIT);
    *duration = GST_BUFFER_DURATION(buffer);
    GstCaps *caps = gst_sample_get_caps(sample);
    if (caps && gst_caps_get_size(caps)) {
        const GstStructure *s = gst_caps_get_structure(caps, 0);
        gst_structure_get_int(s, "width", width);
        gst_structure_get_int(s, "height", height);
    }
    return 1;
}
static void release_sample(GstSample *sample, GstMapInfo *map) {
    gst_buffer_unmap(gst_sample_get_buffer(sample), map);
    gst_sample_unref(sample);
}
static void sample_color(GstSample *sample, guint8 *out) {
    GstCaps *caps = gst_sample_get_caps(sample);
    if (!caps || !gst_caps_get_size(caps)) return;
    const char *name = gst_structure_get_string(gst_caps_get_structure(caps, 0), "colorimetry");
    GstVideoColorimetry color;
    if (!name || !gst_video_colorimetry_from_string(&color, name)) return;
    out[0] = gst_video_color_primaries_to_iso(color.primaries);
    out[1] = gst_video_transfer_function_to_iso(color.transfer);
    out[2] = gst_video_color_matrix_to_iso(color.matrix);
    out[3] = color.range == GST_VIDEO_COLOR_RANGE_0_255 ? 0x20 : 0x10;
}
static char *pipeline_error(GstElement *pipeline) {
    GstBus *bus = gst_element_get_bus(pipeline);
    GstMessage *msg = gst_bus_pop_filtered(bus, GST_MESSAGE_ERROR);
    gst_object_unref(bus);
    if (!msg) return NULL;
    GError *error = NULL;
    gchar *debug = NULL;
    gst_message_parse_error(msg, &error, &debug);
    char *text = g_strdup(error->message);
    g_error_free(error);
    g_free(debug);
    gst_message_unref(msg);
    return text;
}
static int is_appsink(GstElement *element) { return GST_IS_APP_SINK(element); }
static int sink_started(GstElement *sink) {
    GstState state;
    gst_element_get_state(sink, &state, NULL, 0);
    return state >= GST_STATE_PAUSED;
}
static int int_property_range(const char *factory, const char *name, int *min, int *max) {
    gst_init(NULL, NULL);
    GstElement *element = gst_element_factory_make(factory, NULL);
    if (!element) return 0;
    GParamSpec *spec = g_object_class_find_property(G_OBJECT_GET_CLASS(element), name);
    int found = spec && G_IS_PARAM_SPEC_INT(spec) && (spec->flags & G_PARAM_WRITABLE);
    if (found) {
        *min = G_PARAM_SPEC_INT(spec)->minimum;
        *max = G_PARAM_SPEC_INT(spec)->maximum;
    }
    gst_object_unref(element);
    return found;
}
static int request_keyframe(GstElement *sink) {
    return gst_element_send_event(sink,
        gst_video_event_new_upstream_force_key_unit(GST_CLOCK_TIME_NONE, TRUE, 0));
}
static int int_property_value(const char *factory, const char *name, int *value) {
    gst_init(NULL, NULL);
    GstElement *element = gst_element_factory_make(factory, NULL);
    if (!element) return 0;
    GParamSpec *spec = g_object_class_find_property(G_OBJECT_GET_CLASS(element), name);
    int found = spec && G_IS_PARAM_SPEC_INT(spec) && (spec->flags & G_PARAM_READABLE);
    if (found) g_object_get(element, name, value, NULL);
    gst_object_unref(element);
    return found;
}
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"
	"unsafe"
)

type Frame struct {
	Data          []byte
	PTS           time.Duration
	HasPTS        bool
	Duration      time.Duration
	Width, Height int
	Keyframe      bool
	ColorSpace    [4]byte
}

// IntPropertyValue queries device status without starting a capture pipeline.
func IntPropertyValue(factory, property string) (int, bool) {
	f, p := C.CString(factory), C.CString(property)
	defer C.free(unsafe.Pointer(f))
	defer C.free(unsafe.Pointer(p))
	var value C.int
	found := C.int_property_value(f, p, &value)
	return int(value), found != 0
}

// IntPropertyRange inspects the loaded plugin without starting a pipeline.
func IntPropertyRange(factory, property string) (min, max int, ok bool) {
	f := C.CString(factory)
	p := C.CString(property)
	defer C.free(unsafe.Pointer(f))
	defer C.free(unsafe.Pointer(p))
	var lo, hi C.int
	found := C.int_property_range(f, p, &lo, &hi)
	return int(lo), int(hi), found != 0
}

// Run owns the pipeline until cancellation, EOS or error. The description must
// contain an appsink named frames. consume runs synchronously and must return
// promptly. The data is copied into Go memory and may be retained by the caller.
// Each value received from keyframes asks the encoder for an IDR frame; nil
// disables requests.
// No encoded buffers are dropped here: discarding an inter frame can corrupt a
// GOP. Backpressure is bounded by the appsink queue instead.
func Run(ctx context.Context, description string, keyframes <-chan struct{}, consume func(Frame) error) error {
	if consume == nil {
		return errors.New("missing frame consumer")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	desc := C.CString(description)
	defer C.free(unsafe.Pointer(desc))
	var message *C.char
	pipeline := C.open_pipeline(desc, &message)
	if message != nil {
		defer C.g_free(C.gpointer(message))
		return fmt.Errorf("create pipeline: %s", C.GoString(message))
	}
	if pipeline == nil {
		return errors.New("empty pipeline")
	}
	defer C.gst_object_unref(C.gpointer(pipeline))
	name := C.CString("frames")
	defer C.free(unsafe.Pointer(name))
	sink := C.gst_bin_get_by_name((*C.GstBin)(unsafe.Pointer(pipeline)), name)
	if sink == nil {
		return errors.New("pipeline has no frames appsink")
	}
	defer C.gst_object_unref(C.gpointer(sink))
	if C.is_appsink(sink) == 0 {
		return errors.New("frames must be an appsink")
	}
	app := (*C.GstAppSink)(unsafe.Pointer(sink))
	C.gst_app_sink_set_max_buffers(app, 2)
	C.gst_app_sink_set_drop(app, 0)
	// Release DMA-bufs and the hardware encoder even on callback failure.
	defer C.gst_element_set_state(pipeline, C.GST_STATE_NULL)
	if C.gst_element_set_state(pipeline, C.GST_STATE_PLAYING) == C.GST_STATE_CHANGE_FAILURE {
		if msg := C.pipeline_error(pipeline); msg != nil {
			err := fmt.Errorf("start capture: %s", C.GoString(msg))
			C.g_free(C.gpointer(msg))
			return err
		}
		return errors.New("pipeline refused PLAYING state")
	}
	previous := uint64(^uint64(0))
	lastFrame := time.Now()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if msg := C.pipeline_error(pipeline); msg != nil {
			err := fmt.Errorf("capture: %s", C.GoString(msg))
			C.g_free(C.gpointer(msg))
			return err
		}
		select {
		case <-keyframes:
			if C.request_keyframe(sink) == 0 {
				return errors.New("encoder rejected keyframe request")
			}
		default:
		}
		sample := C.gst_app_sink_try_pull_sample(app, C.GstClockTime(100*time.Millisecond))
		if sample == nil {
			// is_eos also returns true before appsink starts. An asynchronous
			// capture-bin startup must not become a spurious encoder failure.
			started := C.sink_started(sink) != 0
			if started && C.gst_app_sink_is_eos(app) != 0 {
				return io.EOF
			}
			if time.Since(lastFrame) >= 3*time.Second {
				return errors.New("capture produced no frames for 3 seconds")
			}
			if !started {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(10 * time.Millisecond):
				}
			}
			continue
		}
		lastFrame = time.Now()
		var mapped C.GstMapInfo
		var pts, duration C.guint64
		var width, height, keyframe C.int
		if C.sample_data(sample, &mapped, &pts, &duration, &width, &height, &keyframe) == 0 {
			C.gst_sample_unref(sample)
			return errors.New("cannot map capture buffer")
		}
		if uint64(mapped.size) > 16*1024*1024 {
			C.release_sample(sample, &mapped)
			return errors.New("capture buffer exceeds 16 MiB")
		}
		frame := Frame{Data: C.GoBytes(unsafe.Pointer(mapped.data), C.int(mapped.size)),
			PTS: time.Duration(pts), HasPTS: uint64(pts) != ^uint64(0), Width: int(width), Height: int(height), Keyframe: keyframe != 0}
		var color [4]C.guint8
		C.sample_color(sample, &color[0])
		for i := range color {
			frame.ColorSpace[i] = byte(color[i])
		}
		frame.Duration = frameDuration(previous, uint64(pts), uint64(duration))
		previous = uint64(pts)
		C.release_sample(sample, &mapped)
		if err := consume(frame); err != nil {
			return err
		}
	}
}
